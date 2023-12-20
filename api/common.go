package api

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ControllerName string

const (
	ServiceInstanceController                  ControllerName = "ServiceInstance"
	ServiceBindingController                   ControllerName = "ServiceBinding"
	FinalizerName                              string         = "services.cloud.sap.com/sap-btp-finalizer"
	StaleBindingIDLabel                        string         = "services.cloud.sap.com/stale"
	StaleBindingRotationOfLabel                string         = "services.cloud.sap.com/rotationOf"
	ForceRotateAnnotation                      string         = "services.cloud.sap.com/forceRotate"
	PreventDeletion                            string         = "services.cloud.sap.com/preventDeletion"
	UseInstanceMetadataNameInSecret            string         = "services.cloud.sap.com/useInstanceMetadataName"
	IgnoreNonTransientErrorAnnotation          string         = "services.cloud.sap.com/ignoreNonTransientError"
	IgnoreNonTransientErrorTimestampAnnotation string         = "services.cloud.sap.com/ignoreNonTransientErrorTimestamp"
)

type HTTPStatusCodeError struct {
	// StatusCode is the HTTP status code returned by the broker.
	StatusCode int
	// ErrorMessage is a machine-readable error string that may be returned by the broker.
	ErrorMessage *string
	// Description is a human-readable description of the error that may be returned by the broker.
	Description *string
	// ResponseError is set to the error that occurred when unmarshalling a response body from the broker.
	ResponseError error
}

func (e HTTPStatusCodeError) Error() string {
	errorMessage := ""
	description := ""

	if e.ErrorMessage != nil {
		errorMessage = *e.ErrorMessage
	}
	if e.Description != nil {
		description = *e.Description
	}
	return fmt.Sprintf("BrokerError:%s, Status: %d, Description: %s", errorMessage, e.StatusCode, description)
}

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
	GetAnnotations() map[string]string
	SetAnnotations(map[string]string)
}
