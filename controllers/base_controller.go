package controllers

import (
	"context"
	"net/http"

	"fmt"

	"github.com/SAP/sap-btp-service-operator/api"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/SAP/sap-btp-service-operator/internal/config"
	"github.com/SAP/sap-btp-service-operator/internal/secrets"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	CreateFailed      = "CreateFailed"
	UpdateFailed      = "UpdateFailed"
	DeleteFailed      = "DeleteFailed"
	Failed            = "Failed"
	ShareFailed       = "ShareFailed"
	ShareSucceeded    = "ShareSucceeded"
	ShareNotSupported = "ShareNotSupported"
	UnShareFailed     = "UnShareFailed"
	UnShareSucceeded  = "UnShareSucceeded"

	Blocked = "Blocked"
	Unknown = "Unknown"

	// Cred Rotation
	CredPreparing = "Preparing"
	CredRotating  = "Rotating"
)

type LogKey struct {
}

type BaseReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	SMClient       func() sm.Client
	Config         config.Config
	SecretResolver *secrets.SecretResolver
	Recorder       record.EventRecorder
}

func GetLogger(ctx context.Context) logr.Logger {
	return ctx.Value(LogKey{}).(logr.Logger)
}

func (r *BaseReconciler) getSMClient(ctx context.Context, object api.SAPBTPResource) (sm.Client, error) {
	if r.SMClient != nil {
		return r.SMClient(), nil
	}
	log := GetLogger(ctx)

	secret, err := r.SecretResolver.GetSecretForResource(ctx, object.GetNamespace(), secrets.SAPBTPOperatorSecretName)
	if err != nil {
		return nil, err
	}

	secretData := secret.Data
	cfg := &sm.ClientConfig{
		ClientID:       string(secretData["clientid"]),
		ClientSecret:   string(secretData["clientsecret"]),
		URL:            string(secretData["sm_url"]),
		TokenURL:       string(secretData["tokenurl"]),
		TokenURLSuffix: string(secretData["tokenurlsuffix"]),
		SSLDisabled:    false,
	}

	if len(cfg.ClientSecret) == 0 {
		tls, err := r.SecretResolver.GetSecretForResource(ctx, object.GetNamespace(), secrets.SAPBTPOperatorTLSSecretName)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		if tls != nil {
			cfg.TLSCertKey = string(tls.Data[v1.TLSCertKey])
			cfg.TLSPrivateKey = string(tls.Data[v1.TLSPrivateKeyKey])
			log.Info("found tls configuration", "client", cfg.ClientID)
		}
	}

	cl, err := sm.NewClient(ctx, cfg, nil)
	return cl, err
}

func (r *BaseReconciler) removeFinalizer(ctx context.Context, object api.SAPBTPResource, finalizerName string) error {
	log := GetLogger(ctx)
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

func (r *BaseReconciler) updateStatus(ctx context.Context, object api.SAPBTPResource) error {
	return r.Status().Update(ctx, object)
}

func (r *BaseReconciler) init(ctx context.Context, obj api.SAPBTPResource) error {
	obj.SetReady(metav1.ConditionFalse)
	setInProgressConditions(smClientTypes.CREATE, "Pending", obj)
	if err := r.updateStatus(ctx, obj); err != nil {
		return err
	}
	return nil
}

func getConditionReason(opType smClientTypes.OperationCategory, state smClientTypes.OperationState) string {
	switch state {
	case smClientTypes.SUCCEEDED:
		if opType == smClientTypes.CREATE {
			return Created
		} else if opType == smClientTypes.UPDATE {
			return Updated
		} else if opType == smClientTypes.DELETE {
			return Deleted
		} else {
			return Finished
		}
	case smClientTypes.INPROGRESS, smClientTypes.PENDING:
		if opType == smClientTypes.CREATE {
			return CreateInProgress
		} else if opType == smClientTypes.UPDATE {
			return UpdateInProgress
		} else if opType == smClientTypes.DELETE {
			return DeleteInProgress
		} else {
			return InProgress
		}
	case smClientTypes.FAILED:
		if opType == smClientTypes.CREATE {
			return CreateFailed
		} else if opType == smClientTypes.UPDATE {
			return UpdateFailed
		} else if opType == smClientTypes.DELETE {
			return DeleteFailed
		} else {
			return Failed
		}
	}

	return Unknown
}

func setInProgressConditions(operationType smClientTypes.OperationCategory, message string, object api.SAPBTPResource) {
	if len(message) == 0 {
		if operationType == smClientTypes.CREATE {
			message = fmt.Sprintf("%s is being created", object.GetControllerName())
		} else if operationType == smClientTypes.UPDATE {
			message = fmt.Sprintf("%s is being updated", object.GetControllerName())
		} else if operationType == smClientTypes.DELETE {
			message = fmt.Sprintf("%s is being deleted", object.GetControllerName())
		}
	}

	conditions := object.GetConditions()
	if len(conditions) > 0 {
		meta.RemoveStatusCondition(&conditions, api.ConditionFailed)
	}
	lastOpCondition := metav1.Condition{
		Type:               api.ConditionSucceeded,
		Status:             metav1.ConditionFalse,
		Reason:             getConditionReason(operationType, smClientTypes.INPROGRESS),
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
}

func setSuccessConditions(operationType smClientTypes.OperationCategory, object api.SAPBTPResource) {
	var message string
	if operationType == smClientTypes.CREATE {
		message = fmt.Sprintf("%s provisioned successfully", object.GetControllerName())
	} else if operationType == smClientTypes.UPDATE {
		message = fmt.Sprintf("%s updated successfully", object.GetControllerName())
	} else if operationType == smClientTypes.DELETE {
		message = fmt.Sprintf("%s deleted successfully", object.GetControllerName())
	}

	conditions := object.GetConditions()
	if len(conditions) > 0 {
		meta.RemoveStatusCondition(&conditions, api.ConditionFailed)
	}
	lastOpCondition := metav1.Condition{
		Type:               api.ConditionSucceeded,
		Status:             metav1.ConditionTrue,
		Reason:             getConditionReason(operationType, smClientTypes.SUCCEEDED),
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
}

func setCredRotationInProgressConditions(reason, message string, object api.SAPBTPResource) {
	if len(message) == 0 {
		message = reason
	}
	conditions := object.GetConditions()
	credRotCondition := metav1.Condition{
		Type:               api.ConditionCredRotationInProgress,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, credRotCondition)
	object.SetConditions(conditions)
}

func setFailureConditions(operationType smClientTypes.OperationCategory, errorMessage string, object api.SAPBTPResource) {
	var message string
	if operationType == smClientTypes.CREATE {
		message = fmt.Sprintf("%s create failed: %s", object.GetControllerName(), errorMessage)
	} else if operationType == smClientTypes.UPDATE {
		message = fmt.Sprintf("%s update failed: %s", object.GetControllerName(), errorMessage)
	} else if operationType == smClientTypes.DELETE {
		message = fmt.Sprintf("%s deletion failed: %s", object.GetControllerName(), errorMessage)
	}

	var reason string
	if operationType != Unknown {
		reason = getConditionReason(operationType, smClientTypes.FAILED)
	} else {
		reason = object.GetConditions()[0].Reason
	}

	conditions := object.GetConditions()
	lastOpCondition := metav1.Condition{
		Type:               api.ConditionSucceeded,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)

	failedCondition := metav1.Condition{
		Type:               api.ConditionFailed,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, failedCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
}

// blocked condition marks to the user that action from his side is required, this is considered as in progress operation
func setBlockedCondition(message string, object api.SAPBTPResource) {
	setInProgressConditions(Unknown, message, object)
	lastOpCondition := meta.FindStatusCondition(object.GetConditions(), api.ConditionSucceeded)
	lastOpCondition.Reason = Blocked
}

func isDelete(object metav1.ObjectMeta) bool {
	return !object.DeletionTimestamp.IsZero()
}

func isTransientError(ctx context.Context, err error) bool {
	log := GetLogger(ctx)
	if smError, ok := err.(*sm.ServiceManagerError); ok {
		log.Info(fmt.Sprintf("SM returned error status code %d", smError.StatusCode))
		return smError.StatusCode == http.StatusTooManyRequests || smError.StatusCode == http.StatusServiceUnavailable ||
			smError.StatusCode == http.StatusGatewayTimeout || smError.StatusCode == http.StatusNotFound || smError.StatusCode == http.StatusBadGateway
	}
	return false
}

func (r *BaseReconciler) markAsNonTransientError(ctx context.Context, operationType smClientTypes.OperationCategory, nonTransientErr error, object api.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	setFailureConditions(operationType, nonTransientErr.Error(), object)
	if operationType != smClientTypes.DELETE {
		log.Info(fmt.Sprintf("operation %s of %s encountered a non transient error %s, giving up operation :(", operationType, object.GetControllerName(), nonTransientErr.Error()))
	}
	object.SetObservedGeneration(object.GetGeneration())
	err := r.updateStatus(ctx, object)
	if err != nil {
		return ctrl.Result{}, err
	}
	if operationType == smClientTypes.DELETE {
		return ctrl.Result{}, nonTransientErr
	}
	return ctrl.Result{}, nil
}

func (r *BaseReconciler) markAsTransientError(ctx context.Context, operationType smClientTypes.OperationCategory, transientErr error, object api.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	if smError, ok := transientErr.(*sm.ServiceManagerError); ok && smError.StatusCode != http.StatusTooManyRequests {
		setInProgressConditions(operationType, transientErr.Error(), object)
		log.Info(fmt.Sprintf("operation %s of %s encountered a transient error %s, retrying operation :)", operationType, object.GetControllerName(), transientErr.Error()))
		if err := r.updateStatus(ctx, object); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, transientErr
}

func isInProgress(object api.SAPBTPResource) bool {
	conditions := object.GetConditions()
	return meta.IsStatusConditionPresentAndEqual(conditions, api.ConditionSucceeded, metav1.ConditionFalse) &&
		!meta.IsStatusConditionPresentAndEqual(conditions, api.ConditionFailed, metav1.ConditionTrue)
}

func isFailed(resource api.SAPBTPResource) bool {
	if len(resource.GetConditions()) == 0 {
		return false
	}
	return meta.IsStatusConditionPresentAndEqual(resource.GetConditions(), api.ConditionFailed, metav1.ConditionTrue) ||
		(resource.GetConditions()[0].Status == metav1.ConditionFalse &&
			resource.GetConditions()[0].Type == api.ConditionSucceeded &&
			resource.GetConditions()[0].Reason == Blocked)
}

func getReadyCondition(object api.SAPBTPResource) metav1.Condition {
	status := metav1.ConditionFalse
	reason := "NotProvisioned"
	if object.GetReady() == metav1.ConditionTrue {
		status = metav1.ConditionTrue
		reason = "Provisioned"
	}

	return metav1.Condition{Type: api.ConditionReady, Status: status, Reason: reason, ObservedGeneration: object.GetGeneration()}
}
