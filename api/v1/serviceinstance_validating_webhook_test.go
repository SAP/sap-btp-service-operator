package v1

import (
	"github.com/SAP/sap-btp-service-operator/api/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Instance Webhook Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
	})

	Context("Validate Delete", func() {
		When("service instance is marked as prevent deletion", func() {
			It("should return error from webhook", func() {
				instance.Annotations = map[string]string{
					common.PreventDeletion: "true",
				}
				_, err := instance.ValidateDelete(nil, instance)
				Expect(err).To(HaveOccurred())
			})
		})

		When("service instance is not marked as prevent deletion", func() {
			It("should not return error from webhook", func() {
				_, err := instance.ValidateDelete(nil, instance)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("service instance prevent deletion annotation is not set with true", func() {
			It("should not return error from webhook", func() {
				instance.Annotations = map[string]string{
					common.PreventDeletion: "not-true",
				}
				_, err := instance.ValidateDelete(nil, instance)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
