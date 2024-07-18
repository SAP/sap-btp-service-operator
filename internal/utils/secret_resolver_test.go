package utils

import (
	"github.com/SAP/sap-btp-service-operator/internal/config"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = Describe("Secrets Resolver", func() {
	var (
		expectedClientID string
		secret           *corev1.Secret
	)

	createSecret := func(namePrefix string, namespace string) *corev1.Secret {
		var name string
		if namePrefix == "" {
			name = SAPBTPOperatorSecretName
		} else {
			name = fmt.Sprintf("%s-%s", namePrefix, SAPBTPOperatorSecretName)
		}
		By(fmt.Sprintf("Creating secret with name %s", name))

		expectedClientID = uuid.New().String()
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"clientid":     []byte(expectedClientID),
				"clientsecret": []byte("client-secret"),
				"url":          []byte("https://some.url"),
				"tokenurl":     []byte("https://token.url"),
			},
		}

		err := k8sClient.Create(ctx, newSecret)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: newSecret.Name, Namespace: newSecret.Namespace}, newSecret)
			if err != nil {
				return false
			}
			return len(newSecret.Data) > 0
		}, timeout, interval).Should(BeTrue())

		return newSecret
	}

	validateSecretResolved := func() {
		resolvedSecret, err := secretsClient.getSecretForResource(ctx, testNamespace, SAPBTPOperatorSecretName)
		Expect(err).ToNot(HaveOccurred())
		Expect(resolvedSecret).ToNot(BeNil())
		Expect(string(resolvedSecret.Data["clientid"])).To(Equal(expectedClientID))
	}

	validateSecretNotResolved := func() {
		_, err := secretsClient.getSecretForResource(ctx, testNamespace, SAPBTPOperatorSecretName)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	}

	BeforeEach(func() {
		InitializeSecretsClient(k8sClient, nil, config.Config{
			ManagementNamespace: managementNamespace,
			ReleaseNamespace:    managementNamespace,
		})
	})

	AfterEach(func() {
		if secret != nil {
			err := k8sClient.Delete(ctx, secret)
			Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
		}
	})

	Context("Secret doesn't exist", func() {
		It("should fail to resolve the secret", func() {
			validateSecretNotResolved()
		})
	})

	Context("Secret in resource namespace", func() {
		BeforeEach(func() {
			secret = createSecret("", testNamespace)
		})
		Context("Namespace secrets enabled", func() {
			BeforeEach(func() {
				secretsClient.EnableNamespaceSecrets = true
			})
			It("should resolve the secret", func() {
				fmt.Printf("secret %v", secret)
				validateSecretResolved()
			})
		})

		Context("Namespace secrets disabled", func() {
			It("should fail to resolve the secret", func() {
				validateSecretNotResolved()
			})

			When("secret for resource namespace exists in management namespace", func() {
				var anotherSecret *corev1.Secret

				BeforeEach(func() {
					anotherSecret = createSecret(testNamespace, managementNamespace)
				})

				AfterEach(func() {
					err := k8sClient.Delete(ctx, anotherSecret)
					Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
				})

				It("should resolve the secret", func() {
					validateSecretResolved()
				})
			})

			When("cluster exists in management namespace", func() {
				var anotherSecret *corev1.Secret

				BeforeEach(func() {
					anotherSecret = createSecret("", managementNamespace)
				})

				AfterEach(func() {
					err := k8sClient.Delete(ctx, anotherSecret)
					Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
				})

				It("should resolve the secret", func() {
					validateSecretResolved()
				})
			})
		})
	})

	Context("Secret for resource namespace is in management namespace", func() {
		BeforeEach(func() {
			secret = createSecret(testNamespace, managementNamespace)
		})

		It("should resolve the secret", func() {
			validateSecretResolved()
		})
	})

	Context("Cluster secret is in management namespace", func() {
		BeforeEach(func() {
			secret = createSecret("", managementNamespace)
		})

		It("should resolve the secret", func() {
			validateSecretResolved()
		})
	})

	Context("btp access secret in management namespace", func() {
		subaccountID := "12345"
		BeforeEach(func() {
			secret = createSecret(subaccountID, managementNamespace)
		})

		It("should resolve the secret", func() {
			resolvedSecret, err := secretsClient.getSecretFromManagementNamespace(ctx, secret.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(resolvedSecret).ToNot(BeNil())
			Expect(string(resolvedSecret.Data["clientid"])).To(Equal(expectedClientID))
		})
	})

	Context("getWithClientFallback unit", func() {
		When("LimitedCacheEnabled is false", func() {
			It("should not fallback to NonCachedClient", func() {
				secretsClient.NonCachedClient = nil // we will get nil pointer in case of fallback
				err := secretsClient.getWithClientFallback(ctx, types.NamespacedName{Name: "some-name", Namespace: testNamespace}, &corev1.Secret{})
				Expect(err).To(HaveOccurred())
			})
		})

		When("LimitedCacheEnabled is true", func() {
			It("should fallback to NonCachedClient and fail if not found", func() {
				secretsClient.LimitedCacheEnabled = true
				secretsClient.NonCachedClient = fake.NewFakeClient()
				err := secretsClient.getWithClientFallback(ctx, types.NamespacedName{Name: "some-name", Namespace: testNamespace}, &corev1.Secret{})
				Expect(err).To(HaveOccurred())
			})

			It("should fallback to NonCachedClient and succeed if found", func() {
				secretsClient.LimitedCacheEnabled = true
				fakeClient := fake.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "some-name",
						Namespace: testNamespace,
					},
					Data: map[string][]byte{
						"key": []byte("value"),
					},
				})
				secretsClient.NonCachedClient = fakeClient
				searchedSecret := &corev1.Secret{}
				err := secretsClient.getWithClientFallback(ctx, types.NamespacedName{Name: "some-name", Namespace: testNamespace}, searchedSecret)
				Expect(err).ToNot(HaveOccurred())
				Expect(searchedSecret.Data).To(HaveKey("key"))
			})
		})
	})
})
