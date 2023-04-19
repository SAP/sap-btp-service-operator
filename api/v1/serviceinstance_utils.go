package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	conditions := serviceInstance.GetConditions()
	for _, condition := range conditions {
		if condition.Type == api.ConditionSharing {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}
