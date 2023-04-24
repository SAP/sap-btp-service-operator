package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ControllerName string

const (
	ServiceInstanceController   ControllerName = "ServiceInstance"
	ServiceBindingController    ControllerName = "ServiceBinding"
	FinalizerName               string         = "services.cloud.sap.com/sap-btp-finalizer"
	StaleBindingIDLabel         string         = "services.cloud.sap.com/stale"
	StaleBindingRotationOfLabel string         = "services.cloud.sap.com/rotationOf"
	ForceRotateAnnotation       string         = "services.cloud.sap.com/forceRotate"
)

const (
	// ConditionSucceeded represents whether the last operation CREATE/UPDATE/DELETE was successful.
	ConditionSucceeded = "Succeeded"

	// ConditionFailed represents information about a final failure that should not be retried.
	ConditionFailed = "Failed"

	// ConditionReady represents if the resource ready for usage.
	ConditionReady = "Ready"

	// ConditionCredRotationInProgress represents if cred rotation is in progress
	ConditionCredRotationInProgress = "CredRotationInProgress"

	// ConditionPendingTermination resource is waiting for termination pre-conditions
	ConditionPendingTermination = "PendingTermination"

	// ConditionShared represents information about the instance share situation
	ConditionShared = "Shared"
)

// +kubebuilder:object:generate=false
type SAPBTPResource interface {
	client.Object
	SetConditions([]metav1.Condition)
	GetConditions() []metav1.Condition
	GetControllerName() ControllerName
	GetParameters() *runtime.RawExtension
	GetStatus() interface{}
	SetStatus(status interface{})
	GetObservedGeneration() int64
	SetObservedGeneration(int64)
	DeepClone() SAPBTPResource
	SetReady(metav1.ConditionStatus)
	GetReady() metav1.ConditionStatus
}
