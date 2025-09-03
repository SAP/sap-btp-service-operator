package utils

import (
	"encoding/json"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smclientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"k8s.io/apimachinery/pkg/api/meta"

	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Controller Util", func() {

	Context("normalize credentials", func() {
		var credentialsJSON json.RawMessage

		BeforeEach(func() {
			credentialsJSON = []byte(`{"keyStr":"val", "keyBool":true,"keyNum":8,"keyJSON":{"a":"b"}}`)
		})

		It("should normalize correctly", func() {
			res, metadata, err := NormalizeCredentials(credentialsJSON)
			str := SecretMetadataProperty{
				Name:   "keyStr",
				Format: string(TEXT),
			}
			boolean := SecretMetadataProperty{
				Name:   "keyBool",
				Format: string(JSON),
			}
			num := SecretMetadataProperty{
				Name:   "keyNum",
				Format: string(JSON),
			}
			json := SecretMetadataProperty{
				Name:   "keyJSON",
				Format: string(JSON),
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res)).To(Equal(4))
			Expect(metadata).To(ContainElements(str, boolean, num, json))
		})

	})

	Context("SliceContains", func() {
		It("slice contains", func() {
			slice := []string{"element1", "element2", "element3"}
			Expect(SliceContains(slice, "element2")).To(BeTrue())
		})

		It("slice doesn't contain", func() {
			slice := []string{"element1", "element2", "element3"}
			Expect(SliceContains(slice, "element4")).To(BeFalse())
		})

		It("empty slice", func() {
			slice := []string{}
			Expect(SliceContains(slice, "element1")).To(BeFalse())
		})
	})

	Context("RemoveAnnotations tests", func() {
		var resource *v1.ServiceBinding
		BeforeEach(func() {
			resource = getBinding()
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})
		AfterEach(func() {
			err := k8sClient.Delete(ctx, resource)
			Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
		})
		When("a single key is removed", func() {
			BeforeEach(func() {
				resource.Annotations = map[string]string{"key1": "value1", "key2": "value2"}
			})

			It("should not return an error and remove the annotation", func() {
				err := RemoveAnnotations(ctx, k8sClient, resource, "key1")
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.GetAnnotations()).To(Equal(map[string]string{"key2": "value2"}))
			})
		})

		When("multiple keys are removed", func() {
			BeforeEach(func() {
				resource.Annotations = map[string]string{"key1": "value1", "key2": "value2"}
			})

			It("should not return an error and remove annotations", func() {
				err := RemoveAnnotations(ctx, k8sClient, resource, "key1", "key2")
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.GetAnnotations()).To(BeEmpty())
			})
		})

		When("non-existent key is removed", func() {
			BeforeEach(func() {
				resource.Annotations = map[string]string{"key1": "value1", "key2": "value2"}
			})

			It("should not return an error", func() {
				err := RemoveAnnotations(ctx, k8sClient, resource, "key3")
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.GetAnnotations()).To(Equal(map[string]string{"key1": "value1", "key2": "value2"}))
			})
		})

		When("annotations are empty", func() {
			BeforeEach(func() {
				resource.Annotations = map[string]string{}
			})

			It("should not return an error", func() {
				err := RemoveAnnotations(ctx, k8sClient, resource, "key1")
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.GetAnnotations()).To(BeEmpty())
			})
		})
	})

	Context("build user info test ", func() {

		It("should return empty with nil UserInfo", func() {
			Expect(BuildUserInfo(ctx, nil)).To(Equal(""))
		})

		It("should return correct UserInfo string with valid UserInfo", func() {
			got := BuildUserInfo(ctx, &authv1.UserInfo{Username: "user1", UID: "1"})
			expected := `{"username":"user1","uid":"1"}`
			Expect(got).To(Equal(expected))
		})
	})

	Context("ParseNamespacedName", func() {
		It("should return correct namespace and name", func() {
			nsName, err := ParseNamespacedName(types.NamespacedName{
				Namespace: "namespace",
				Name:      "name",
			}.String())
			Expect(err).ToNot(HaveOccurred())
			Expect(nsName.Namespace).To(Equal("namespace"))
			Expect(nsName.Name).To(Equal("name"))
		})

		It("should return error if not a valid namespaced name", func() {
			_, err := ParseNamespacedName("namespaceName")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid format: expected 'namespace/name"))
		})
	})

	Context("AddWatchForSecretIfNeeded", func() {
		It("should add the watch label to the secret if it is missing", func() {
			// Create a fake client

			// Create a secret without the watch label
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
			}
			err := k8sClient.Create(ctx, secret)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-secret", Namespace: "default"}, secret)
			Expect(err).ToNot(HaveOccurred())

			// Call the function
			err = AddWatchForSecretIfNeeded(ctx, k8sClient, secret, "123")
			Expect(err).ToNot(HaveOccurred())

			// Get the updated secret
			updatedSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-secret", Namespace: "default"}, updatedSecret)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedSecret.Finalizers[0]).To(Equal(common.FinalizerName))
			Expect(updatedSecret.Annotations[common.WatchSecretAnnotation+"123"]).To(Equal("true"))

		})
	})

	Context("handleRateLimitError", func() {
		var resource common.SAPBTPResource
		BeforeEach(func() {
			resource = getBinding()
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should update the condition to in progress", func() {
			headers := map[string][]string{"Retry-After": {"2024-11-11 14:59:33 +0000 UTC"}}
			_, err := handleRateLimitError(ctx, k8sClient, resource, smclientTypes.CREATE, &sm.ServiceManagerError{ResponseHeaders: headers})
			Expect(err).ToNot(HaveOccurred())
			conds := resource.GetConditions()
			Expect(len(conds)).To(Equal(2))
			succeededCond := meta.FindStatusCondition(conds, common.ConditionSucceeded)
			Expect(succeededCond).NotTo(BeNil())
			Expect(succeededCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(succeededCond.Reason).To(Equal(common.CreateInProgress))
		})
	})
})
