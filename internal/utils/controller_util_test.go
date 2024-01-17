package utils

import (
	"encoding/json"
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"time"
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

	Context("ShouldIgnoreNonTransient", func() {
		var (
			instance *v1.ServiceInstance
			logger   = logf.Log.WithName("test-logger")
		)

		BeforeEach(func() {
			instance = &v1.ServiceInstance{}
		})

		It("should return false if no ignore annotation", func() {
			Expect(ShouldIgnoreNonTransient(logger, instance, time.Hour)).To(BeTrue())
		})
		It("should return false if time exceeded", func() {
			instance.Status.FirstErrorTimestamp = time.Now().Add(-2 * time.Hour)
			Expect(ShouldIgnoreNonTransient(logger, instance, time.Hour)).To(BeFalse())
		})
		It("should return true if time not exceeded", func() {
			instance.Status.FirstErrorTimestamp = time.Now()
			Expect(ShouldIgnoreNonTransient(logger, instance, time.Hour)).To(BeTrue())
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

	Context("IsTransientError", func() {
		var instance *sm.ServiceManagerError
		var log logr.Logger
		BeforeEach(func() {
			log = GetLogger(ctx)
		})
		When("400 status code", func() {
			BeforeEach(func() {
				instance = &sm.ServiceManagerError{
					StatusCode: 400,
				}
			})

			It("should not be transient error", func() {
				Expect(IsTransientError(instance, log)).To(BeFalse())
			})
		})

		When("internal server error status code", func() {
			BeforeEach(func() {
				instance = &sm.ServiceManagerError{
					StatusCode: 500,
				}
			})

			It("should be non transient error", func() {
				Expect(IsTransientError(instance, log)).To(BeFalse())
			})
		})

		When("concurrent operation error", func() {
			BeforeEach(func() {
				instance = &sm.ServiceManagerError{
					StatusCode: http.StatusUnprocessableEntity,
					ErrorType:  "ConcurrentOperationInProgress",
				}
			})

			It("should be transient error", func() {
				Expect(IsTransientError(instance, log)).To(BeTrue())
			})
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
})
