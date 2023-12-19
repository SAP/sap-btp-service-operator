package v1

import (
	"fmt"
	"github.com/SAP/sap-btp-service-operator/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("Service Instance Webhook Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
	})

	Context("Validate Update", func() {
		When("btpAccessCredentialsSecret changed", func() {
			It("should fail", func() {
				instance := getInstance()
				instance.Spec.BTPAccessCredentialsSecret = ""
				newInstance := getInstance()
				newInstance.Spec.BTPAccessCredentialsSecret = "new-secret"
				_, err := newInstance.ValidateUpdate(instance)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("changing the btpAccessCredentialsSecret for an existing instance is not allowed"))
			})
		})
		When("service instance IgnoreNonTransientErrorTimestampAnnotation annotation is not set with not a date", func() {
			It("should return error from webhook", func() {
				instance.Annotations = map[string]string{
					api.IgnoreNonTransientErrorAnnotation:          "true",
					api.IgnoreNonTransientErrorTimestampAnnotation: "true",
				}
				_, err := instance.ValidateUpdate(instance)
				Expect(err).To(HaveOccurred())

			})
		})
		When("service instance IgnoreNonTransientErrorTimestampAnnotation annotation is not set with future date", func() {
			It("should return error from webhook", func() {
				instance.Annotations = map[string]string{
					api.IgnoreNonTransientErrorAnnotation:          "true",
					api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Add(48 * time.Hour).Format(time.RFC3339),
				}
				_, err := instance.ValidateUpdate(instance)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("Validate Delete", func() {
		When("service instance is marked as prevent deletion", func() {
			It("should return error from webhook", func() {
				instance.Annotations = map[string]string{
					api.PreventDeletion: "true",
				}
				_, err := instance.ValidateDelete()
				Expect(err).To(HaveOccurred())
			})
		})

		When("service instance is not marked as prevent deletion", func() {
			It("should not return error from webhook", func() {
				_, err := instance.ValidateDelete()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("service instance prevent deletion annotation is not set with true", func() {
			It("should not return error from webhook", func() {
				instance.Annotations = map[string]string{
					api.PreventDeletion: "not-true",
				}
				_, err := instance.ValidateDelete()
				Expect(err).ToNot(HaveOccurred())
			})
		})
		Context("Validate Create", func() {
			When("service instance IgnoreNonTransientErrorTimestampAnnotation annotation is not set with not a date", func() {
				It("should not return error from webhook", func() {
					instance.Annotations = map[string]string{
						api.IgnoreNonTransientErrorAnnotation:          "true",
						api.IgnoreNonTransientErrorTimestampAnnotation: "true",
					}
					_, err := instance.ValidateCreate()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(fmt.Sprintf(annotationNotValidTimestampError, api.IgnoreNonTransientErrorTimestampAnnotation)))

				})
			})
			When("service instance IgnoreNonTransientErrorTimestampAnnotation annotation is not set with future date", func() {
				It("should not return error from webhook", func() {
					instance.Annotations = map[string]string{
						api.IgnoreNonTransientErrorAnnotation:          "true",
						api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Add(48 * time.Hour).Format(time.RFC3339),
					}
					_, err := instance.ValidateCreate()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(fmt.Sprintf(annotationInFutureError, api.IgnoreNonTransientErrorTimestampAnnotation)))

				})
			})
		})
	})

	Context("validateNonTransientTimestampAnnotation", func() {
		It("fails when not a valid date", func() {
			annotation := map[string]string{
				api.IgnoreNonTransientErrorAnnotation:          "true",
				api.IgnoreNonTransientErrorTimestampAnnotation: "true",
			}
			instance.SetAnnotations(annotation)
			err := instance.validateNonTransientTimestampAnnotation()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is not a valid timestamp"))
		})

		It("fails when a future date", func() {
			annotation := map[string]string{
				api.IgnoreNonTransientErrorAnnotation:          "true",
				api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Add(48 * time.Hour).Format(time.RFC3339),
			}
			instance.SetAnnotations(annotation)
			err := instance.validateNonTransientTimestampAnnotation()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf(annotationInFutureError, api.IgnoreNonTransientErrorTimestampAnnotation)))
		})

		It("succeeds when timestamp is valid and in the past", func() {
			annotation := map[string]string{
				api.IgnoreNonTransientErrorAnnotation:          "true",
				api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Format(time.RFC3339),
			}
			instance.SetAnnotations(annotation)
			err := instance.validateNonTransientTimestampAnnotation()
			Expect(err).NotTo(HaveOccurred())
		})
		//
		//It("validate annotation exist and valid", func() {
		//	annotation := map[string]string{
		//		api.IgnoreNonTransientErrorAnnotation:          "true",
		//		api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Format(time.RFC3339),
		//	}
		//	instance.SetAnnotations(annotation)
		//	exist := utils.IsIgnoreNonTransientAnnotationExistAndValid(serviceinstancelog, instance, time.Hour)
		//	Expect(exist).To(BeTrue())
		//})
		//
		//It("validate timeout for Ignore Non Transient Error Annotation", func() {
		//	annotation := map[string]string{
		//		api.IgnoreNonTransientErrorAnnotation:          "true",
		//		api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Truncate(48 * time.Hour).Format(time.RFC3339),
		//	}
		//	instance.SetAnnotations(annotation)
		//	exist := utils.IsIgnoreNonTransientAnnotationExistAndValid(serviceinstancelog, instance, time.Hour)
		//	Expect(exist).To(BeFalse())
		//})
		//
		//It("validate annotation not exist", func() {
		//	annotation := map[string]string{
		//		api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Format(time.RFC3339),
		//	}
		//	instance.SetAnnotations(annotation)
		//	exist := utils.IsIgnoreNonTransientAnnotationExistAndValid(serviceinstancelog, instance, time.Hour)
		//	Expect(exist).To(BeFalse())
		//})
	})
})
