package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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

	Created        = "Created"
	Updated        = "Updated"
	Deleted        = "Deleted"
	Finished       = "Finished"
	Provisioned    = "Provisioned"
	NotProvisioned = "NotProvisioned"

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

func (r *BaseReconciler) getSMClient(ctx context.Context, object api.SAPBTPResource, subaccountID string) (sm.Client, error) {
	if r.SMClient != nil {
		return r.SMClient(), nil
	}
	log := GetLogger(ctx)

	secret, err := r.SecretResolver.GetSecretForResource(ctx, object.GetNamespace(), secrets.SAPBTPOperatorSecretName, subaccountID)
	if err != nil {
		return nil, err
	}

	clientConfig := &sm.ClientConfig{
		ClientID:       string(secret.Data["clientid"]),
		ClientSecret:   string(secret.Data["clientsecret"]),
		URL:            string(secret.Data["sm_url"]),
		TokenURL:       string(secret.Data["tokenurl"]),
		TokenURLSuffix: string(secret.Data["tokenurlsuffix"]),
		SSLDisabled:    false,
	}

	if len(clientConfig.ClientID) == 0 || len(clientConfig.URL) == 0 || len(clientConfig.TokenURL) == 0 {
		log.Info("credentials secret found but did not contain all the required data")
		return nil, fmt.Errorf("invalid Service-Manager credentials, contact your cluster administrator")
	}

	if len(clientConfig.ClientSecret) == 0 {
		tlsSecret, err := r.SecretResolver.GetSecretForResource(ctx, object.GetNamespace(), secrets.SAPBTPOperatorTLSSecretName, subaccountID)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		if tlsSecret == nil || len(tlsSecret.Data) == 0 || len(tlsSecret.Data[v1.TLSCertKey]) == 0 || len(tlsSecret.Data[v1.TLSPrivateKeyKey]) == 0 {
			log.Info("clientsecret not found in SM credentials, and tls secret is invalid")
			return nil, fmt.Errorf("invalid Service-Manager credentials, contact your cluster administrator")
		}

		log.Info("found tls configuration")
		clientConfig.TLSCertKey = string(tlsSecret.Data[v1.TLSCertKey])
		clientConfig.TLSPrivateKey = string(tlsSecret.Data[v1.TLSPrivateKeyKey])
	}

	cl, err := sm.NewClient(ctx, clientConfig, nil)
	return cl, err
}

func (r *BaseReconciler) removeFinalizer(ctx context.Context, object api.SAPBTPResource, finalizerName string) error {
	log := GetLogger(ctx)
	if controllerutil.ContainsFinalizer(object, finalizerName) {
		log.Info(fmt.Sprintf("removing finalizer %s", finalizerName))
		controllerutil.RemoveFinalizer(object, finalizerName)
		if err := r.Client.Update(ctx, object); err != nil {
			if err := r.Client.Get(ctx, apimachinerytypes.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object); err != nil {
				return client.IgnoreNotFound(err)
			}
			controllerutil.RemoveFinalizer(object, finalizerName)
			if err := r.Client.Update(ctx, object); err != nil {
				return fmt.Errorf("failed to remove the finalizer '%s'. Error: %v", finalizerName, err)
			}
		}
		log.Info(fmt.Sprintf("removed finalizer %s from %s", finalizerName, object.GetControllerName()))
		return nil
	}
	return nil
}

func (r *BaseReconciler) updateStatus(ctx context.Context, object api.SAPBTPResource) error {
	log := GetLogger(ctx)
	log.Info(fmt.Sprintf("updating %s status", object.GetObjectKind().GroupVersionKind().Kind))
	return r.Client.Status().Update(ctx, object)
}

func (r *BaseReconciler) init(ctx context.Context, obj api.SAPBTPResource) error {
	obj.SetReady(metav1.ConditionFalse)
	setInProgressConditions(ctx, smClientTypes.CREATE, "Pending", obj)
	return r.updateStatus(ctx, obj)
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

func setInProgressConditions(ctx context.Context, operationType smClientTypes.OperationCategory, message string, object api.SAPBTPResource) {
	log := GetLogger(ctx)
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
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
	log.Info(fmt.Sprintf("setting inProgress conditions: reason: %s, message:%s, generation: %d", lastOpCondition.Reason, message, object.GetGeneration()))
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
		ObservedGeneration: object.GetObservedGeneration(),
	}
	readyCondition := metav1.Condition{
		Type:               api.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             Provisioned,
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, readyCondition)

	object.SetConditions(conditions)
	object.SetReady(metav1.ConditionTrue)
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
		ObservedGeneration: object.GetObservedGeneration(),
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
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)

	failedCondition := metav1.Condition{
		Type:               api.ConditionFailed,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, failedCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
}

// blocked condition marks to the user that action from his side is required, this is considered as in progress operation
func setBlockedCondition(ctx context.Context, message string, object api.SAPBTPResource) {
	setInProgressConditions(ctx, Unknown, message, object)
	lastOpCondition := meta.FindStatusCondition(object.GetConditions(), api.ConditionSucceeded)
	lastOpCondition.Reason = Blocked
}

func isMarkedForDeletion(object metav1.ObjectMeta) bool {
	return !object.DeletionTimestamp.IsZero()
}

func (r *BaseReconciler) isTransientError(smError *sm.ServiceManagerError, log logr.Logger) bool {
	statusCode := smError.GetStatusCode()
	log.Info(fmt.Sprintf("SM returned error with status code %d", statusCode))
	return isTransientStatusCode(statusCode) || isConcurrentOperationError(smError)
}

func isConcurrentOperationError(smError *sm.ServiceManagerError) bool {
	// service manager returns 422 for resources that have another operation in progress
	// in this case 422 status code is transient
	return smError.StatusCode == http.StatusUnprocessableEntity && smError.ErrorType == "ConcurrentOperationInProgress"
}

func isTransientStatusCode(StatusCode int) bool {
	return StatusCode == http.StatusTooManyRequests ||
		StatusCode == http.StatusServiceUnavailable ||
		StatusCode == http.StatusGatewayTimeout ||
		StatusCode == http.StatusBadGateway ||
		StatusCode == http.StatusNotFound
}

func (r *BaseReconciler) handleError(ctx context.Context, operationType smClientTypes.OperationCategory, err error, resource api.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	var smError *sm.ServiceManagerError
	ok := errors.As(err, &smError)
	if !ok {
		log.Info("unable to cast error to SM error, will be treated as non transient")
		return r.markAsNonTransientError(ctx, operationType, err.Error(), resource)
	}
	if r.isTransientError(smError, log) || shouldIgnoreNonTransient(log, resource, r.Config.IgnoreNonTransientTimeout) {
		return r.markAsTransientError(ctx, operationType, smError, resource)
	}

	return r.markAsNonTransientError(ctx, operationType, smError.Error(), resource)
}

func (r *BaseReconciler) markAsNonTransientError(ctx context.Context, operationType smClientTypes.OperationCategory, errMsg string, object api.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	setFailureConditions(operationType, errMsg, object)
	if operationType != smClientTypes.DELETE {
		log.Info(fmt.Sprintf("operation %s of %s encountered a non transient error %s, giving up operation :(", operationType, object.GetControllerName(), errMsg))
	}
	object.SetObservedGeneration(object.GetGeneration())
	err := r.updateStatus(ctx, object)
	if err != nil {
		return ctrl.Result{}, err
	}
	if operationType == smClientTypes.DELETE {
		return ctrl.Result{}, fmt.Errorf(errMsg)
	}
	return ctrl.Result{}, nil
}

func (r *BaseReconciler) markAsTransientError(ctx context.Context, operationType smClientTypes.OperationCategory, err error, object api.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	//DO NOT REMOVE - 429 error is not reflected to the status
	if smError, ok := err.(*sm.ServiceManagerError); !ok || smError.StatusCode != http.StatusTooManyRequests {
		setInProgressConditions(ctx, operationType, err.Error(), object)
		log.Info(fmt.Sprintf("operation %s of %s encountered a transient error %s, retrying operation :)", operationType, object.GetControllerName(), err.Error()))
		if updateErr := r.updateStatus(ctx, object); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
	}

	return ctrl.Result{}, err
}

func (r *BaseReconciler) removeAnnotation(ctx context.Context, object api.SAPBTPResource, keys ...string) error {
	log := GetLogger(ctx)
	annotations := object.GetAnnotations()
	shouldUpdate := false
	if annotations != nil {
		for _, key := range keys {
			if _, ok := annotations[key]; ok {
				log.Info(fmt.Sprintf("deleting annotation with key %s", key))
				delete(annotations, key)
				shouldUpdate = true
			}
		}
		if shouldUpdate {
			object.SetAnnotations(annotations)
			return r.Client.Update(ctx, object)
		}
	}
	return nil
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
	reason := NotProvisioned
	if object.GetReady() == metav1.ConditionTrue {
		status = metav1.ConditionTrue
		reason = Provisioned
	}

	return metav1.Condition{Type: api.ConditionReady, Status: status, Reason: reason}
}
