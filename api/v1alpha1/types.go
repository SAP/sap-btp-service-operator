package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ControllerName string

const (
	ServiceInstanceController ControllerName = "ServiceInstance"
	ServiceBindingController  ControllerName = "ServiceBinding"
)

const (
	// ConditionReady represents that a given resource is in ready state.
	ConditionReady = "Ready"

	// ConditionFailed represents information about a final failure that should not be retried.
	ConditionFailed = "Failed"
)

// +kubebuilder:object:generate=false
type SAPCPResource interface {
	client.Object
	SetConditions([]metav1.Condition)
	GetConditions() []metav1.Condition
	GetControllerName() ControllerName
	GetParameters() *runtime.RawExtension
	GetStatus() interface{}
	SetStatus(status interface{})
	GetObservedGeneration() int64
	SetObservedGeneration(int64)
	DeepClone() SAPCPResource
}
