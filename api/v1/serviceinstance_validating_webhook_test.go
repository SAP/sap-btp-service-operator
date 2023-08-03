package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Binding Webhook Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
	})

	Context("Validate Delete", func() {
		When("service instance is marked as prevent deletion", func() {
			It("should not delete the instance", func() {
				instance.Annotations = map[string]string{
					api.PreventDeletion: "true",
				}
				_, err := instance.ValidateDelete()
				Expect(err).To(HaveOccurred())
			})
		})

		When("service instance is not marked as prevent deletion", func() {
			It("should delete the instance", func() {
				_, err := instance.ValidateDelete()
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
