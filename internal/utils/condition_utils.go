package utils

import (
	"context"
	"fmt"
	"net/http"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func InitConditions(ctx context.Context, k8sClient client.Client, obj common.SAPBTPResource) error {
	obj.SetReady(metav1.ConditionFalse)
	SetInProgressConditions(ctx, smClientTypes.CREATE, "Pending", obj)
	return UpdateStatus(ctx, k8sClient, obj)
}

func GetConditionReason(opType smClientTypes.OperationCategory, state smClientTypes.OperationState) string {
	switch state {
	case smClientTypes.SUCCEEDED:
		if opType == smClientTypes.CREATE {
			return common.Created
		} else if opType == smClientTypes.UPDATE {
			return common.Updated
		} else if opType == smClientTypes.DELETE {
			return common.Deleted
		} else {
			return common.Finished
		}
	case smClientTypes.INPROGRESS, smClientTypes.PENDING:
		if opType == smClientTypes.CREATE {
			return common.CreateInProgress
		} else if opType == smClientTypes.UPDATE {
			return common.UpdateInProgress
		} else if opType == smClientTypes.DELETE {
			return common.DeleteInProgress
		} else {
			return common.InProgress
		}
	case smClientTypes.FAILED:
		if opType == smClientTypes.CREATE {
			return common.CreateFailed
		} else if opType == smClientTypes.UPDATE {
			return common.UpdateFailed
		} else if opType == smClientTypes.DELETE {
			return common.DeleteFailed
		} else {
			return common.Failed
		}
	}

	return common.Unknown
}

func SetInProgressConditions(ctx context.Context, operationType smClientTypes.OperationCategory, message string, object common.SAPBTPResource) {
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
		meta.RemoveStatusCondition(&conditions, common.ConditionFailed)
	}
	lastOpCondition := metav1.Condition{
		Type:               common.ConditionSucceeded,
		Status:             metav1.ConditionFalse,
		Reason:             GetConditionReason(operationType, smClientTypes.INPROGRESS),
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
	log.Info(fmt.Sprintf("setting inProgress conditions: reason: %s, message:%s, generation: %d", lastOpCondition.Reason, message, object.GetGeneration()))
}

func SetSuccessConditions(operationType smClientTypes.OperationCategory, object common.SAPBTPResource) {
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
		meta.RemoveStatusCondition(&conditions, common.ConditionFailed)
	}
	lastOpCondition := metav1.Condition{
		Type:               common.ConditionSucceeded,
		Status:             metav1.ConditionTrue,
		Reason:             GetConditionReason(operationType, smClientTypes.SUCCEEDED),
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	readyCondition := metav1.Condition{
		Type:               common.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             common.Provisioned,
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)
	meta.SetStatusCondition(&conditions, readyCondition)

	object.SetConditions(conditions)
	object.SetReady(metav1.ConditionTrue)
}

func SetCredRotationInProgressConditions(reason, message string, object common.SAPBTPResource) {
	if len(message) == 0 {
		message = reason
	}
	conditions := object.GetConditions()
	credRotCondition := metav1.Condition{
		Type:               common.ConditionCredRotationInProgress,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, credRotCondition)
	object.SetConditions(conditions)
}

func SetFailureConditions(operationType smClientTypes.OperationCategory, errorMessage string, object common.SAPBTPResource) {
	var message string
	if operationType == smClientTypes.CREATE {
		message = fmt.Sprintf("%s create failed: %s", object.GetControllerName(), errorMessage)
	} else if operationType == smClientTypes.UPDATE {
		message = fmt.Sprintf("%s update failed: %s", object.GetControllerName(), errorMessage)
	} else if operationType == smClientTypes.DELETE {
		message = fmt.Sprintf("%s deletion failed: %s", object.GetControllerName(), errorMessage)
	}

	var reason string
	if operationType != common.Unknown {
		reason = GetConditionReason(operationType, smClientTypes.FAILED)
	} else {
		reason = object.GetConditions()[0].Reason
	}

	conditions := object.GetConditions()
	lastOpCondition := metav1.Condition{
		Type:               common.ConditionSucceeded,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, lastOpCondition)

	failedCondition := metav1.Condition{
		Type:               common.ConditionFailed,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: object.GetObservedGeneration(),
	}
	meta.SetStatusCondition(&conditions, failedCondition)
	meta.SetStatusCondition(&conditions, getReadyCondition(object))

	object.SetConditions(conditions)
}

func MarkAsNonTransientError(ctx context.Context, k8sClient client.Client, operationType smClientTypes.OperationCategory, errMsg string, object common.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	SetFailureConditions(operationType, errMsg, object)
	if operationType != smClientTypes.DELETE {
		log.Info(fmt.Sprintf("operation %s of %s encountered a non transient error %s, giving up operation :(", operationType, object.GetControllerName(), errMsg))
	}
	object.SetObservedGeneration(object.GetGeneration())
	err := UpdateStatus(ctx, k8sClient, object)
	if err != nil {
		return ctrl.Result{}, err
	}
	if operationType == smClientTypes.DELETE {
		return ctrl.Result{}, fmt.Errorf(errMsg)
	}
	return ctrl.Result{}, nil
}

func MarkAsTransientError(ctx context.Context, k8sClient client.Client, operationType smClientTypes.OperationCategory, err error, object common.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	//DO NOT REMOVE - 429 error is not reflected to the status
	if smError, ok := err.(*sm.ServiceManagerError); !ok || smError.StatusCode != http.StatusTooManyRequests {
		SetInProgressConditions(ctx, operationType, err.Error(), object)
		log.Info(fmt.Sprintf("operation %s of %s encountered a transient error %s, retrying operation :)", operationType, object.GetControllerName(), err.Error()))
		if updateErr := UpdateStatus(ctx, k8sClient, object); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
	}

	return ctrl.Result{}, err
}

// blocked condition marks to the user that action from his side is required, this is considered as in progress operation
func SetBlockedCondition(ctx context.Context, message string, object common.SAPBTPResource) {
	SetInProgressConditions(ctx, common.Unknown, message, object)
	lastOpCondition := meta.FindStatusCondition(object.GetConditions(), common.ConditionSucceeded)
	lastOpCondition.Reason = common.Blocked
}

func IsInProgress(object common.SAPBTPResource) bool {
	conditions := object.GetConditions()
	return meta.IsStatusConditionPresentAndEqual(conditions, common.ConditionSucceeded, metav1.ConditionFalse) &&
		!meta.IsStatusConditionPresentAndEqual(conditions, common.ConditionFailed, metav1.ConditionTrue)
}

func IsFailed(resource common.SAPBTPResource) bool {
	if len(resource.GetConditions()) == 0 {
		return false
	}
	return meta.IsStatusConditionPresentAndEqual(resource.GetConditions(), common.ConditionFailed, metav1.ConditionTrue) ||
		(resource.GetConditions()[0].Status == metav1.ConditionFalse &&
			resource.GetConditions()[0].Type == common.ConditionSucceeded &&
			resource.GetConditions()[0].Reason == common.Blocked)
}

func getReadyCondition(object common.SAPBTPResource) metav1.Condition {
	status := metav1.ConditionFalse
	reason := common.NotProvisioned
	if object.GetReady() == metav1.ConditionTrue {
		status = metav1.ConditionTrue
		reason = common.Provisioned
	}

	return metav1.Condition{Type: common.ConditionReady, Status: status, Reason: reason}
}
