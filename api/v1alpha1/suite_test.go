package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SAPBTP Resource Suite")
}

func getBinding() *ServiceBinding {
	return &ServiceBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "services.cloud.sap.com/v1alpha1",
			Kind:       "ServiceBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-binding-1",
			Namespace: "namespace-1",
		},
		Spec: ServiceBindingSpec{
			ServiceInstanceName: "service-instance-1",
			ExternalName:        "my-service-binding-1",
			Parameters:          &runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
			ParametersFrom: []ParametersFromSource{
				{
					SecretKeyRef: &SecretKeyReference{
						Name: "param-secret",
						Key:  "secret-parameter",
					},
				},
			},
		},

		Status: ServiceBindingStatus{},
	}
}

func getInstance() *ServiceInstance {
	return &ServiceInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "services.cloud.sap.com/v1alpha1",
			Kind:       "ServiceInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-instance-1",
			Namespace: "namespace-1",
		},
		Spec: ServiceInstanceSpec{
			ServiceOfferingName: "service-offering-1",
			ServicePlanName:     "service-plan-name-1",
			ServicePlanID:       "service-plan-id-1",
			ExternalName:        "my-service-instance-1",
			Parameters:          &runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
			ParametersFrom: []ParametersFromSource{
				{
					SecretKeyRef: &SecretKeyReference{
						Name: "param-secret",
						Key:  "secret-parameter",
					},
				},
			},
		},

		Status: ServiceInstanceStatus{},
	}
}
