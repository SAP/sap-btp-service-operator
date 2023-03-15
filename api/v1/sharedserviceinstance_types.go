/*
Copyright 2023.

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

package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	v1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SharedServiceInstanceSpec defines the desired state of SharedServiceInstance
type SharedServiceInstanceSpec struct {
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

	// UserInfo contains information about the user that last modified this
	// instance. This field is set by the API server and not settable by the
	// end-user. User-provided values for this field are not saved.
	// +optional
	UserInfo *v1.UserInfo `json:"userInfo,omitempty"`

	// Parameters for the binding.
	//
	// The Parameters field is NOT secret or secured in any way and should
	// NEVER be used to hold sensitive information. To set parameters that
	// contain secret information, you should ALWAYS store that information
	// in a Secret and use the ParametersFrom field.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// SharedServiceInstanceStatus defines the observed state of SharedServiceInstance
type SharedServiceInstanceStatus struct {
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

	// Service binding conditions
	Conditions []metav1.Condition `json:"conditions"`

	// Last generation that was acted on
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Indicates whether binding is ready for usage
	Ready metav1.ConditionStatus `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".spec.serviceInstanceName",name="Instance",type=string
// +kubebuilder:printcolumn:JSONPath=".status.conditions[0].reason",name="Status",type=string
// +kubebuilder:printcolumn:JSONPath=".status.ready",name="Ready",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name="Age",type=date
// +kubebuilder:printcolumn:JSONPath=".status.bindingID",name="ID",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.conditions[0].message",name="Message",type=string,priority=1

// SharedServiceInstance is the Schema for the sharedserviceinstances API
type SharedServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SharedServiceInstanceSpec   `json:"spec,omitempty"`
	Status SharedServiceInstanceStatus `json:"status,omitempty"`
}

func (ssi *SharedServiceInstance) GetParameters() *runtime.RawExtension {
	return ssi.Spec.Parameters
}

// SharedServiceInstanceList contains a list of SharedServiceInstance
type SharedServiceInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SharedServiceInstance `json:"items"`
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *SharedServiceInstanceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (ssi *SharedServiceInstance) GetConditions() []metav1.Condition {
	return ssi.Status.Conditions
}

func (ssi *SharedServiceInstance) SetConditions(conditions []metav1.Condition) {
	ssi.Status.Conditions = conditions
}

func (ssi *SharedServiceInstance) GetControllerName() api.ControllerName {
	return api.SharedServiceInstanceController
}

func (ssi *SharedServiceInstance) GetStatus() interface{} {
	return ssi.Status
}

func (ssi *SharedServiceInstance) SetStatus(status interface{}) {
	ssi.Status = status.(SharedServiceInstanceStatus)
}

func (ssi *SharedServiceInstance) GetObservedGeneration() int64 {
	return ssi.Status.ObservedGeneration
}

func (ssi *SharedServiceInstance) SetObservedGeneration(newObserved int64) {
	ssi.Status.ObservedGeneration = newObserved
}

func (ssi *SharedServiceInstance) DeepClone() api.SAPBTPResource {
	return ssi.DeepCopy()
}

func (ssi *SharedServiceInstance) GetReady() metav1.ConditionStatus {
	return ssi.Status.Ready
}

func (ssi *SharedServiceInstance) SetReady(ready metav1.ConditionStatus) {
	ssi.Status.Ready = ready
}

func init() {
	SchemeBuilder.Register(&SharedServiceInstance{}, &SharedServiceInstanceList{})
}
