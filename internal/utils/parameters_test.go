package utils

import (
	"github.com/SAP/sap-btp-service-operator/internal/config"

	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Parameters", func() {
	Describe("BuildSMRequestParameters", func() {
		It("handles empty parameters", func() {
			var parametersFrom []v1.ParametersFromSource
			parameters := (*runtime.RawExtension)(nil)

			rawParam, secrets, err := BuildSMRequestParameters("", parameters, parametersFrom)

			Expect(err).To(BeNil())
			Expect(rawParam).To(BeNil())
			Expect(len(secrets)).To(BeZero())
		})
		It("handles parameters from source", func() {
			var parametersFrom []v1.ParametersFromSource
			parameters := &runtime.RawExtension{
				Raw: []byte(`{"key":"value"}`),
			}

			rawParam, secrets, err := BuildSMRequestParameters("", parameters, parametersFrom)

			Expect(err).To(BeNil())
			Expect(rawParam).To(Equal([]byte(`{"key":"value"}`)))
			Expect(len(secrets)).To(BeZero())
		})
		It("handles parameters from source with secrets", func() {
			// Setup
			namespace := "test-namespace"
			parameters := &runtime.RawExtension{Raw: []byte(`{"param1":"value1"}`)}
			secretData := map[string][]byte{"secret-parameter": []byte(`{"param2":"value2"}`)}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "param-secret",
					Namespace: namespace,
				},
				Data: secretData,
			}
			parametersFrom := []v1.ParametersFromSource{
				{
					SecretKeyRef: &v1.SecretKeyReference{
						Name: "param-secret",
						Key:  "secret-parameter",
					},
				},
			}

			// Create a fake client with the secret
			k8sClient := fake.NewClientBuilder().WithObjects(secret).Build()

			// Initialize the secrets client
			InitializeSecretsClient(k8sClient, k8sClient, config.Config{
				ManagementNamespace:    "management-namespace",
				ReleaseNamespace:       "release-namespace",
				EnableNamespaceSecrets: true,
				EnableLimitedCache:     true,
			})

			// Test
			parametersRaw, secretsSet, err := BuildSMRequestParameters(namespace, parameters, parametersFrom)

			// Assertions
			Expect(err).To(BeNil())
			expectedParams := map[string]interface{}{
				"param1": "value1",
				"param2": "value2",
			}
			rawParameters, err := MarshalRawParameters(expectedParams)
			Expect(err).To(BeNil())
			Expect(parametersRaw).To(Equal(rawParameters))
			Expect(len(secretsSet)).To(Equal(1))
			Expect(secretsSet[string(secret.UID)]).To(Equal(secret))
		})
	})
})
