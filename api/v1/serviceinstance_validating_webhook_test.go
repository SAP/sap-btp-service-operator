package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
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
	})

	Context("Validate Update", func() {
		When("service instance subaccountID changed", func() {
			It("should return error from webhook", func() {
				instance := getInstanceWithSubaccountID()
				newInstance := getInstanceWithSubaccountID()
				newInstance.Spec.SubaccountID = "12345"
				_, err := newInstance.ValidateUpdate(instance)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("subaccountID spec field can not be changed"))
			})
		})
	})

	Context("Validate Create", func() {
		When("service instance subaccountID changed", func() {
			It("should return error from webhook", func() {
				instance := getInstanceWithSubaccountID()
				instance.SetEnableMUltipleSubaccounts(false)
				_, err := instance.ValidateCreate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("using multiple subaccounts is disabled"))
			})
		})
	})
})
