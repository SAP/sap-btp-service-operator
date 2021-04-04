package controllers

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/client-go/tools/record"

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

	Created  = "Created"
	Updated  = "Updated"
	Deleted  = "Deleted"
	Finished = "Finished"

	CreateInProgress = "CreateInProgress"
	UpdateInProgress = "UpdateInProgress"
	DeleteInProgress = "DeleteInProgress"
	InProgress       = "InProgress"

	CreateFailed = "CreateFailed"
	UpdateFailed = "UpdateFailed"
	DeleteFailed = "DeleteFailed"
	Failed       = "Failed"

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
	Recorder       record.EventRecorder
}

func (r *BaseReconciler) getSMClient(ctx context.Context, object servicesv1alpha1.SAPBTPResource) (sm.Client, error) {
	if r.SMClient != nil {
		return r.SMClient(), nil
	}

	secret, err := r.SecretResolver.GetSecretForResource(ctx, object.GetNamespace())
	if err != nil {
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
	obj.SetReady(metav1.ConditionFalse)
	setInProgressConditions(smTypes.CREATE, "Pending", obj)
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
		} else {
			return Finished
		}
	case smTypes.IN_PROGRESS, smTypes.PENDING:
		if opType == smTypes.CREATE {
			return CreateInProgress
		} else if opType == smTypes.UPDATE {
			return UpdateInProgress
		} else if opType == smTypes.DELETE {
			return DeleteInProgress
		} else {
			return InProgress
		}
	case smTypes.FAILED:
		if opType == smTypes.CREATE {
			return CreateFailed
		} else if opType == smTypes.UPDATE {
			return UpdateFailed
		} else if opType == smTypes.DELETE {
			return DeleteFailed
		} else {
			return Failed
		}
	}

	return Unknown
}

func setInProgressConditions(operationType smTypes.OperationCategory, message string, object servicesv1alpha1.SAPBTPResource) {
	if len(message) == 0 {
		if operationType == smTypes.CREATE {
			message = fmt.Sprintf("%s is being created", object.GetControllerName())
		} else if operationType == smTypes.UPDATE {
			message = fmt.Sprintf("%s is being updated", object.GetControllerName())
		} else if operationType == smTypes.DELETE {
			message = fmt.Sprintf("%s is being deleted", object.GetControllerName())
		}
	}

	conditions := object.GetConditions()
	if len(conditions) > 0 {
		meta.RemoveStatusCondition(&conditions, servicesv1alpha1.ConditionFailed)
	}
	lastOpCondition := metav1.Condition{
		Type:               servicesv1alpha1.ConditionSucceeded,
		Status:             metav1.ConditionFalse,
		Reason:             getConditionReason(operationType, smTypes.IN_PROGRESS),
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

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
	lastOpCondition := metav1.Condition{
		Type:               servicesv1alpha1.ConditionSucceeded,
		Status:             metav1.ConditionTrue,
		Reason:             getConditionReason(operationType, smTypes.SUCCEEDED),
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

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
	lastOpCondition := metav1.Condition{
		Type:               servicesv1alpha1.ConditionSucceeded,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)

	failedCondition := metav1.Condition{
		Type:               servicesv1alpha1.ConditionFailed,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, failedCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
}

//blocked condition marks to the user that action from his side is required, this is considered as in progress operation
func setBlockedCondition(message string, object servicesv1alpha1.SAPBTPResource) {
	setInProgressConditions(Unknown, message, object)
	lastOpCondition := meta.FindStatusCondition(object.GetConditions(), servicesv1alpha1.ConditionSucceeded)
	lastOpCondition.Reason = Blocked
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

func (r *BaseReconciler) markAsNonTransientError(ctx context.Context, operationType smTypes.OperationCategory, nonTransientErr error, object servicesv1alpha1.SAPBTPResource, log logr.Logger) (ctrl.Result, error) {
	setFailureConditions(operationType, nonTransientErr.Error(), object)
	if operationType != smTypes.DELETE {
		log.Info(fmt.Sprintf("operation %s of %s encountered a non transient error %s, giving up operation :(", operationType, object.GetControllerName(), nonTransientErr.Error()))
	}
	object.SetObservedGeneration(object.GetGeneration())
	err := r.updateStatusWithRetries(ctx, object, log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if operationType == smTypes.DELETE {
		return ctrl.Result{}, nonTransientErr
	}
	return ctrl.Result{}, nil
}

func (r *BaseReconciler) markAsTransientError(ctx context.Context, operationType smTypes.OperationCategory, transientErr error, object servicesv1alpha1.SAPBTPResource, log logr.Logger) (ctrl.Result, error) {
	setInProgressConditions(operationType, transientErr.Error(), object)
	log.Info(fmt.Sprintf("operation %s of %s encountered a transient error %s, retrying operation :)", operationType, object.GetControllerName(), transientErr.Error()))
	if err := r.updateStatusWithRetries(ctx, object, log); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, transientErr
}

func isInProgress(object servicesv1alpha1.SAPBTPResource) bool {
	conditions := object.GetConditions()
	return meta.IsStatusConditionPresentAndEqual(conditions, servicesv1alpha1.ConditionSucceeded, metav1.ConditionFalse) &&
		!meta.IsStatusConditionPresentAndEqual(conditions, servicesv1alpha1.ConditionFailed, metav1.ConditionTrue)
}

func getReadyCondition(object servicesv1alpha1.SAPBTPResource) metav1.Condition {
	status := metav1.ConditionFalse
	reason := "ProvisionFailed"
	if object.GetReady() == metav1.ConditionTrue {
		status = metav1.ConditionTrue
		reason = "Provisioned"
	}

	return metav1.Condition{Type: servicesv1alpha1.ConditionReady, Status: status, Reason: reason}
}
