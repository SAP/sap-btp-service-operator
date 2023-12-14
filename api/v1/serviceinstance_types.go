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

package v1

import (
	"fmt"
	"time"

	"github.com/SAP/sap-btp-service-operator/api"
	"github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceInstanceSpec defines the desired state of ServiceInstance
type ServiceInstanceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The name of the service offering
	// +kubebuilder:validation:MinLength=1
	ServiceOfferingName string `json:"serviceOfferingName"`

	// The name of the service plan
	// +kubebuilder:validation:MinLength=1
	ServicePlanName string `json:"servicePlanName"`

	// The dataCenter in case service offering and plan name exist in other data center and not on main
	// +optional
	DataCenter string `json:"dataCenter,omitempty"`

	// The plan ID in case service offering and plan name are ambiguous
	// +optional
	ServicePlanID string `json:"servicePlanID,omitempty"`

	// The name of the instance in Service Manager
	ExternalName string `json:"externalName,omitempty"`

	// Indicates the desired shared state
	// +optional
	// +kubebuilder:default={}
	Shared *bool `json:"shared,omitempty"`

	// Provisioning parameters for the instance.
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

	// List of custom tags describing the ServiceInstance, will be copied to `ServiceBinding` secret in the key called `tags`.
	// +optional
	CustomTags []string `json:"customTags,omitempty"`

	// UserInfo contains information about the user that last modified this
	// instance. This field is set by the API server and not settable by the
	// end-user. User-provided values for this field are not saved.
	// +optional
	UserInfo *v1.UserInfo `json:"userInfo,omitempty"`

	// The name of the btp access credentials secret
	BTPAccessCredentialsSecret string `json:"btpAccessCredentialsSecret,omitempty"`
}

// ServiceInstanceStatus defines the observed state of ServiceInstance
type ServiceInstanceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The generated ID of the instance, will be automatically filled once the instance is created
	// +optional
	InstanceID string `json:"instanceID,omitempty"`

	// Tags describing the ServiceInstance as provided in service catalog, will be copied to `ServiceBinding` secret in the key called `tags`.
	Tags []string `json:"tags,omitempty"`

	// URL of ongoing operation for the service instance
	OperationURL string `json:"operationURL,omitempty"`

	// The operation type (CREATE/UPDATE/DELETE) for ongoing operation
	OperationType types.OperationCategory `json:"operationType,omitempty"`

	// Service instance conditions
	Conditions []metav1.Condition `json:"conditions"`

	// Last generation that was acted on
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Indicates whether instance is ready for usage
	Ready metav1.ConditionStatus `json:"ready,omitempty"`

	// HashedSpec is the hashed spec without the shared property
	HashedSpec string `json:"hashedSpec,omitempty"`

	// The subaccount id of the service instance
	SubaccountID string `json:"subaccountID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".spec.serviceOfferingName",name="Offering",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.servicePlanName",name="Plan",type=string
// +kubebuilder:printcolumn:JSONPath=".spec.shared",name="shared",type=boolean
// +kubebuilder:printcolumn:JSONPath=".spec.dataCenter",name="dataCenter",type=string
// +kubebuilder:printcolumn:JSONPath=".status.conditions[0].reason",name="Status",type=string
// +kubebuilder:printcolumn:JSONPath=".status.ready",name="Ready",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name="Age",type=date
// +kubebuilder:printcolumn:JSONPath=".status.instanceID",name="ID",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.conditions[0].message",name="Message",type=string,priority=1

// ServiceInstance is the Schema for the serviceinstances API
type ServiceInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServiceInstanceSpec   `json:"spec,omitempty"`
	Status            ServiceInstanceStatus `json:"status,omitempty"`
}

func (si *ServiceInstance) GetConditions() []metav1.Condition {
	return si.Status.Conditions
}

func (si *ServiceInstance) SetConditions(conditions []metav1.Condition) {
	si.Status.Conditions = conditions
}

func (si *ServiceInstance) GetControllerName() api.ControllerName {
	return api.ServiceInstanceController
}

func (si *ServiceInstance) GetParameters() *runtime.RawExtension {
	return si.Spec.Parameters
}

func (si *ServiceInstance) GetStatus() interface{} {
	return si.Status
}

func (si *ServiceInstance) SetStatus(status interface{}) {
	si.Status = status.(ServiceInstanceStatus)
}

func (si *ServiceInstance) GetObservedGeneration() int64 {
	return si.Status.ObservedGeneration
}

func (si *ServiceInstance) SetObservedGeneration(newObserved int64) {
	si.Status.ObservedGeneration = newObserved
}

func (si *ServiceInstance) DeepClone() api.SAPBTPResource {
	return si.DeepCopy()
}

func (si *ServiceInstance) GetReady() metav1.ConditionStatus {
	return si.Status.Ready
}

func (si *ServiceInstance) SetReady(ready metav1.ConditionStatus) {
	si.Status.Ready = ready
}
func (si *ServiceInstance) GetAnnotations() map[string]string {
	return si.Annotations
}

func (si *ServiceInstance) SetAnnotations(annotations map[string]string) {
	si.Annotations = annotations
}

// +kubebuilder:object:root=true

// ServiceInstanceList contains a list of ServiceInstance
type ServiceInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceInstance{}, &ServiceInstanceList{})
}

func (si *ServiceInstance) Hub() {}

func (si *ServiceInstance) ShouldBeShared() bool {
	return si.Spec.Shared != nil && *si.Spec.Shared
}

func (si *ServiceInstance) ValidateNonTransientTimestampAnnotation(log logr.Logger) error {

	sinceAnnotation, exist, err := si.GetTimeSinceIgnoreNonTransientAnnotationTimestamp(log)
	if err != nil {
		return err
	}
	if exist && sinceAnnotation < 0 {
		return fmt.Errorf("annotation %s cannot be a future timestamp", api.IgnoreNonTransientErrorTimestampAnnotation)
	}
	return nil
}

func (si *ServiceInstance) IsIgnoreNonTransientAnnotationExistAndValid(log logr.Logger, timeout time.Duration) bool {

	sinceAnnotation, exist, _ := si.GetTimeSinceIgnoreNonTransientAnnotationTimestamp(log)
	if !exist {
		return false
	}
	if sinceAnnotation > timeout {
		log.Info(fmt.Sprintf("timeout reached- consider error to be non transient. since annotation timestamp %s, IgnoreNonTransientTimeout %s", sinceAnnotation, timeout))
		return false
	}
	log.Info(fmt.Sprintf("timeout didn't reached- consider error to be transient. since annotation timestamp %s, IgnoreNonTransientTimeout %s", sinceAnnotation, timeout))
	return true

}

func (si *ServiceInstance) GetTimeSinceIgnoreNonTransientAnnotationTimestamp(log logr.Logger) (time.Duration, bool, error) {
	if si.Annotations != nil {
		if _, ok := si.Annotations[api.IgnoreNonTransientErrorAnnotation]; ok {
			log.Info("ignoreNonTransientErrorAnnotation annotation exist- checking timeout")
			annotationTime, err := time.Parse(time.RFC3339, si.Annotations[api.IgnoreNonTransientErrorTimestampAnnotation])
			if err != nil {
				log.Error(err, fmt.Sprintf("failed to parse %s", api.IgnoreNonTransientErrorTimestampAnnotation))
				return time.Since(time.Now()), false, fmt.Errorf("annotation %s is not a valid timestamp", api.IgnoreNonTransientErrorTimestampAnnotation)
			}
			return time.Since(annotationTime), true, nil
		}
	}
	return time.Since(time.Now()), false, nil
}
