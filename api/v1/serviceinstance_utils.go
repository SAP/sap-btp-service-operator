package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	"k8s.io/apimachinery/pkg/api/meta"
)

func ShouldHandleSharing(ServiceInstance *ServiceInstance) bool {
	newShareState := ServiceInstance.Spec.Shared
	currentShareState := IsInstanceShared(ServiceInstance)
	if newShareState == nil {
		return currentShareState
	}
	if *newShareState && !currentShareState {
		return true
	}
	if !(*newShareState) && currentShareState {
		return true
	}
	return false
}

func IsInstanceShared(serviceInstance *ServiceInstance) bool {
	return meta.IsStatusConditionTrue(serviceInstance.GetConditions(), api.ConditionShared)
}
