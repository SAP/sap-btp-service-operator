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
	FinalizerName             string         = "services.cloud.sap.com/sap-btp-finalizer"
)

const (
	// ConditionLastOpDone represents the status of last operation CREATE/UPDATE/DELETE.
	ConditionLastOpDone = "LastOpDone"

	// ConditionFailed represents information about a final failure that should not be retried.
	ConditionFailed = "Failed"

	// ConditionReady represents if the resource ready for usage.
	ConditionReady = "Ready"
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

// ParametersFromSource represents the source of a set of Parameters
type ParametersFromSource struct {
	// The Secret key to select from.
	// The value must be a JSON object.
	// +optional
	SecretKeyRef *SecretKeyReference `json:"secretKeyRef,omitempty"`
}

// SecretKeyReference references a key of a Secret.
type SecretKeyReference struct {
	// The name of the secret in the pod's namespace to select from.
	Name string `json:"name"`
	// The key of the secret to select from.  Must be a valid secret key.
	Key string `json:"key"`
}
