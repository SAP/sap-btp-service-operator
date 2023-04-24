package v1alpha1

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
			UserInfo: &v1.UserInfo{
				Username: "test-user",
				Groups:   []string{"test-group"},
				Extra:    map[string]v1.ExtraValue{"key": {"val"}},
			},
			CredRotationPolicy: &CredentialsRotationPolicy{
				Enabled:           true,
				RotationFrequency: "1s",
				RotatedBindingTTL: "1s",
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
			UserInfo: &v1.UserInfo{
				Username: "test-user",
				Groups:   []string{"test-group"},
				Extra:    map[string]v1.ExtraValue{"key": {"val"}},
			},
		},

		Status: ServiceInstanceStatus{},
	}
}
