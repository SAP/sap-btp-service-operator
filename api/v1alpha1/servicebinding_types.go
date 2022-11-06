/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	v1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceBindingSpec defines the desired state of ServiceBinding
type ServiceBindingSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The k8s name of the service instance to bind, should be in the namespace of the binding
	// +required
	// +kubebuilder:validation:MinLength=1
	ServiceInstanceName string `json:"serviceInstanceName"`

	// The name of the binding in Service Manager
	// +optional
	ExternalName string `json:"externalName"`

	// SecretName is the name of the secret where credentials will be stored
	// +optional
	SecretName string `json:"secretName"`

	// SecretKey is used as the key inside the secret to store the credentials
	// returned by the broker encoded as json to support complex data structures.
	// If not specified, the credentials returned by the broker will be used
	// directly as the secrets data.
	// +optional
	SecretKey *string `json:"secretKey,omitempty"`

	// SecretRootKey is used as the key inside the secret to store all binding
	// data including credentials returned by the broker and additional info under single key.
	// Convenient way to store whole binding data in single file when using `volumeMounts`.
	// +optional
	SecretRootKey *string `json:"secretRootKey,omitempty"`

	// Parameters for the binding.
	//
	// The Parameters field is NOT secret or secured in any way and should
	// NEVER be used to hold sensitive information. To set parameters that
	// contain secret information, you should ALWAYS store that information
	// in a Secret and use the ParametersFrom field.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// List of sources to populate parameters.
	// If a top-level parameter name exists in multiples sources among
	// `Parameters` and `ParametersFrom` fields, it is
	// considered to be a user error in the specification
	// +optional
	ParametersFrom []ParametersFromSource `json:"parametersFrom,omitempty"`

	// UserInfo contains information about the user that last modified this
	// instance. This field is set by the API server and not settable by the
	// end-user. User-provided values for this field are not saved.
	// +optional
	UserInfo *v1.UserInfo `json:"userInfo,omitempty"`

	// CredentialsRotationPolicy holds automatic credentials rotation configuration.
	// +optional
	CredRotationPolicy *CredentialsRotationPolicy `json:"credentialsRotationPolicy,omitempty"`
}

// ServiceBindingStatus defines the observed state of ServiceBinding
type ServiceBindingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The ID of the instance in SM associated with binding
	// +optional
	InstanceID string `json:"instanceID,omitempty"`

	// The generated ID of the binding, will be automatically filled once the binding is created
	// +optional
	BindingID string `json:"bindingID,omitempty"`

	// URL of ongoing operation for the service binding
	OperationURL string `json:"operationURL,omitempty"`

	// The operation type (CREATE/UPDATE/DELETE) for ongoing operation
	OperationType smClientTypes.OperationCategory `json:"operationType,omitempty"`

	// Service binding conditions
	Conditions []metav1.Condition `json:"conditions"`

	// Last generation that was acted on
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Indicates whether binding is ready for usage
	Ready metav1.ConditionStatus `json:"ready,omitempty"`

	// Indicates when binding secret was rotated
	LastCredentialsRotationTime *metav1.Time `json:"lastCredentialsRotationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.serviceInstanceName",name="Instance",type=string
// +kubebuilder:printcolumn:JSONPath=".status.conditions[0].reason",name="Status",type=string
// +kubebuilder:printcolumn:JSONPath=".status.ready",name="Ready",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name="Age",type=date
// +kubebuilder:printcolumn:JSONPath=".status.bindingID",name="ID",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.conditions[0].message",name="Message",type=string,priority=1

// ServiceBinding is the Schema for the servicebindings API
type ServiceBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceBindingSpec   `json:"spec,omitempty"`
	Status ServiceBindingStatus `json:"status,omitempty"`
}

func (sb *ServiceBinding) GetConditions() []metav1.Condition {
	return sb.Status.Conditions
}

func (sb *ServiceBinding) SetConditions(conditions []metav1.Condition) {
	sb.Status.Conditions = conditions
}

func (sb *ServiceBinding) GetControllerName() api.ControllerName {
	return api.ServiceBindingController
}

func (sb *ServiceBinding) GetParameters() *runtime.RawExtension {
	return sb.Spec.Parameters
}

func (sb *ServiceBinding) GetStatus() interface{} {
	return sb.Status
}

func (sb *ServiceBinding) SetStatus(status interface{}) {
	sb.Status = status.(ServiceBindingStatus)
}

func (sb *ServiceBinding) GetObservedGeneration() int64 {
	return sb.Status.ObservedGeneration
}

func (sb *ServiceBinding) SetObservedGeneration(newObserved int64) {
	sb.Status.ObservedGeneration = newObserved
}

func (sb *ServiceBinding) DeepClone() api.SAPBTPResource {
	return sb.DeepCopy()
}

func (sb *ServiceBinding) GetReady() metav1.ConditionStatus {
	return sb.Status.Ready
}

func (sb *ServiceBinding) SetReady(ready metav1.ConditionStatus) {
	sb.Status.Ready = ready
}

// +kubebuilder:object:root=true

// ServiceBindingList contains a list of ServiceBinding
type ServiceBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceBinding `json:"items"`
}

type CredentialsRotationPolicy struct {
	Enabled bool `json:"enabled"`
	// What frequency to perform binding rotation.
	RotationFrequency string `json:"rotationFrequency,omitempty"`
	// For how long to keep the rotated binding.
	RotatedBindingTTL string `json:"rotatedBindingTTL,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ServiceBinding{}, &ServiceBindingList{})
}
