package controllers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/SAP/sap-btp-service-operator/client/sm"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/api/meta"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"

	smTypes "github.com/Peripli/service-manager/pkg/types"
	servicesv1alpha1 "github.com/SAP/sap-btp-service-operator/api/v1alpha1"
	"github.com/SAP/sap-btp-service-operator/internal/config"
	"github.com/SAP/sap-btp-service-operator/internal/secrets"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	namespaceLabel = "_namespace"
	k8sNameLabel   = "_k8sname"
	clusterIDLabel = "_clusterid"

	Created = "Created"
	Updated = "Updated"
	Deleted = "Deleted"

	CreateInProgress = "CreateInProgress"
	UpdateInProgress = "UpdateInProgress"
	DeleteInProgress = "DeleteInProgress"

	CreateFailed = "CreateFailed"
	UpdateFailed = "UpdateFailed"
	DeleteFailed = "DeleteFailed"

	Blocked = "Blocked"
	Unknown = "Unknown"
)

type BaseReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	SMClient       func() sm.Client
	Config         config.Config
	SecretResolver *secrets.SecretResolver
}

func (r *BaseReconciler) getSMClient(ctx context.Context, log logr.Logger, object servicesv1alpha1.SAPBTPResource) (sm.Client, error) {
	if r.SMClient != nil {
		return r.SMClient(), nil
	}

	secret, err := r.SecretResolver.GetSecretForResource(ctx, object.GetNamespace())
	if err != nil {
		setBlockedCondition("secret not found", object)
		if err := r.updateStatusWithRetries(ctx, object, log); err != nil {
			return nil, err
		}

		return nil, err
	}

	secretData := secret.Data
	cl := sm.NewClient(ctx, &sm.ClientConfig{
		ClientID:     string(secretData["clientid"]),
		ClientSecret: string(secretData["clientsecret"]),
		URL:          string(secretData["url"]),
		TokenURL:     string(secretData["tokenurl"]),
		SSLDisabled:  false,
	}, nil)

	return cl, nil
}

func (r *BaseReconciler) removeFinalizer(ctx context.Context, object servicesv1alpha1.SAPBTPResource, finalizerName string, log logr.Logger) error {
	if controllerutil.ContainsFinalizer(object, finalizerName) {
		controllerutil.RemoveFinalizer(object, finalizerName)
		if err := r.Update(ctx, object); err != nil {
			if err := r.Get(ctx, apimachinerytypes.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object); err != nil {
				return client.IgnoreNotFound(err)
			}
			controllerutil.RemoveFinalizer(object, finalizerName)
			if err := r.Update(ctx, object); err != nil {
				return fmt.Errorf("failed to remove the finalizer '%s'. Error: %v", finalizerName, err)
			}
		}
		log.Info(fmt.Sprintf("removed finalizer %s from %s", finalizerName, object.GetControllerName()))
		return nil
	}
	return nil
}

func (r *BaseReconciler) updateStatusWithRetries(ctx context.Context, object servicesv1alpha1.SAPBTPResource, log logr.Logger) error {
	logFailedAttempt := func(retries int, err error) {
		log.Info(fmt.Sprintf("failed to update status of %s attempt #%v, %s", object.GetControllerName(), retries, err.Error()))
	}

	log.Info(fmt.Sprintf("updating %s status with retries", object.GetControllerName()))
	var err error
	if err = r.Status().Update(ctx, object); err != nil {
		logFailedAttempt(1, err)
		for i := 2; i <= 3; i++ {
			if err = r.updateStatus(ctx, object, log); err == nil {
				break
			}
			logFailedAttempt(i, err)
		}
	}

	if err != nil {
		log.Error(err, fmt.Sprintf("failed to update status of %s giving up!!", object.GetControllerName()))
		return err
	}

	log.Info(fmt.Sprintf("updated %s status in k8s", object.GetControllerName()))
	return nil
}

func (r *BaseReconciler) updateStatus(ctx context.Context, object servicesv1alpha1.SAPBTPResource, log logr.Logger) error {
	status := object.GetStatus()
	if err := r.Get(ctx, apimachinerytypes.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object); err != nil {
		log.Error(err, fmt.Sprintf("failed to fetch latest %s, unable to update status", object.GetControllerName()))
		return err
	}
	clonedObj := object.DeepClone()
	clonedObj.SetStatus(status)
	if err := r.Status().Update(ctx, clonedObj); err != nil {
		return err
	}
	return nil
}

func (r *BaseReconciler) init(ctx context.Context, log logr.Logger, obj servicesv1alpha1.SAPBTPResource) error {
	setInProgressCondition(smTypes.CREATE, "Pending", obj)
	if err := r.updateStatusWithRetries(ctx, obj, log); err != nil {
		return err
	}
	return nil
}

func getConditionReason(opType smTypes.OperationCategory, state smTypes.OperationState) string {
	switch state {
	case smTypes.SUCCEEDED:
		if opType == smTypes.CREATE {
			return Created
		} else if opType == smTypes.UPDATE {
			return Updated
		} else if opType == smTypes.DELETE {
			return Deleted
		}
	case smTypes.IN_PROGRESS, smTypes.PENDING:
		if opType == smTypes.CREATE {
			return CreateInProgress
		} else if opType == smTypes.UPDATE {
			return UpdateInProgress
		} else if opType == smTypes.DELETE {
			return DeleteInProgress
		}
	case smTypes.FAILED:
		if opType == smTypes.CREATE {
			return CreateFailed
		} else if opType == smTypes.UPDATE {
			return UpdateFailed
		} else if opType == smTypes.DELETE {
			return DeleteFailed
		}
	}

	return Unknown
}

func setInProgressCondition(operationType smTypes.OperationCategory, message string, object servicesv1alpha1.SAPBTPResource) {
	var defaultMessage string
	if operationType == smTypes.CREATE {
		defaultMessage = fmt.Sprintf("%s is being created", object.GetControllerName())
	} else if operationType == smTypes.UPDATE {
		defaultMessage = fmt.Sprintf("%s is being updated", object.GetControllerName())
	} else if operationType == smTypes.DELETE {
		defaultMessage = fmt.Sprintf("%s is being deleted", object.GetControllerName())
	}

	if len(message) == 0 {
		message = defaultMessage
	}

	conditions := object.GetConditions()
	if len(conditions) > 0 {
		meta.RemoveStatusCondition(&conditions, servicesv1alpha1.ConditionFailed)
	}
	readyCondition := metav1.Condition{Type: servicesv1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: getConditionReason(operationType, smTypes.IN_PROGRESS), Message: message}
	meta.SetStatusCondition(&conditions, readyCondition)
	object.SetConditions(conditions)
}

func setSuccessConditions(operationType smTypes.OperationCategory, object servicesv1alpha1.SAPBTPResource) {
	var message string
	if operationType == smTypes.CREATE {
		message = fmt.Sprintf("%s provisioned successfully", object.GetControllerName())
	} else if operationType == smTypes.UPDATE {
		message = fmt.Sprintf("%s updated successfully", object.GetControllerName())
	} else if operationType == smTypes.DELETE {
		message = fmt.Sprintf("%s deleted successfully", object.GetControllerName())
	}

	conditions := object.GetConditions()
	if len(conditions) > 0 {
		meta.RemoveStatusCondition(&conditions, servicesv1alpha1.ConditionFailed)
	}
	readyCondition := metav1.Condition{Type: servicesv1alpha1.ConditionReady, Status: metav1.ConditionTrue, Reason: getConditionReason(operationType, smTypes.SUCCEEDED), Message: message}
	meta.SetStatusCondition(&conditions, readyCondition)
	object.SetConditions(conditions)
}

func setFailureConditions(operationType smTypes.OperationCategory, errorMessage string, object servicesv1alpha1.SAPBTPResource) {
	var message string
	if operationType == smTypes.CREATE {
		message = fmt.Sprintf("%s create failed: %s", object.GetControllerName(), errorMessage)
	} else if operationType == smTypes.UPDATE {
		message = fmt.Sprintf("%s update failed: %s", object.GetControllerName(), errorMessage)
	} else if operationType == smTypes.DELETE {
		message = fmt.Sprintf("%s deletion failed: %s", object.GetControllerName(), errorMessage)
	}

	var reason string
	if operationType != Unknown {
		reason = getConditionReason(operationType, smTypes.FAILED)
	} else {
		reason = object.GetConditions()[0].Reason
	}

	conditions := object.GetConditions()
	readyCondition := metav1.Condition{Type: servicesv1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: reason, Message: message}
	meta.SetStatusCondition(&conditions, readyCondition)

	failedCondition := metav1.Condition{Type: servicesv1alpha1.ConditionFailed, Status: metav1.ConditionTrue, Reason: reason, Message: message}
	meta.SetStatusCondition(&conditions, failedCondition)
	object.SetConditions(conditions)
}

//blocked condition marks to the user that action from his side is required, this is considered as in progress operation
func setBlockedCondition(message string, object servicesv1alpha1.SAPBTPResource) {
	conditions := object.GetConditions()
	if len(conditions) > 0 {
		meta.RemoveStatusCondition(&conditions, servicesv1alpha1.ConditionFailed)
	}
	readyBlockedCondition := metav1.Condition{Type: servicesv1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: Blocked, Message: message}
	meta.SetStatusCondition(&conditions, readyBlockedCondition)
	object.SetConditions(conditions)
}

func isDelete(object metav1.ObjectMeta) bool {
	return !object.DeletionTimestamp.IsZero()
}

func isTransientError(err error, log logr.Logger) bool {
	if smError, ok := err.(*sm.ServiceManagerError); ok {
		log.Info(fmt.Sprintf("SM returned error status code %d", smError.StatusCode))
		return smError.StatusCode == http.StatusTooManyRequests || smError.StatusCode == http.StatusServiceUnavailable || smError.StatusCode == http.StatusGatewayTimeout
	}
	return false
}

func (r *BaseReconciler) markAsNonTransientError(ctx context.Context, operationType smTypes.OperationCategory, message string, object servicesv1alpha1.SAPBTPResource, log logr.Logger) (ctrl.Result, error) {
	setFailureConditions(operationType, message, object)
	log.Info(fmt.Sprintf("operation %s of %s encountered a non transient error, giving up operation :(", operationType, object.GetControllerName()))
	object.SetObservedGeneration(object.GetGeneration())
	err := r.updateStatusWithRetries(ctx, object, log)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *BaseReconciler) markAsTransientError(ctx context.Context, operationType smTypes.OperationCategory, message string, object servicesv1alpha1.SAPBTPResource, log logr.Logger) (ctrl.Result, error) {
	setInProgressCondition(operationType, message, object)
	if err := r.updateStatusWithRetries(ctx, object, log); err != nil {
		return ctrl.Result{}, err
	}

	log.Info(fmt.Sprintf("operation %s of %s encountered a transient error, will try again :)", operationType, object.GetControllerName()))
	return ctrl.Result{Requeue: true, RequeueAfter: r.Config.LongPollInterval}, nil
}

func isInProgress(object servicesv1alpha1.SAPBTPResource) bool {
	conditions := object.GetConditions()
	if len(conditions) == 0 {
		return false
	}
	return len(conditions) == 1 &&
		conditions[0].Type == servicesv1alpha1.ConditionReady &&
		conditions[0].Status == metav1.ConditionFalse
}
