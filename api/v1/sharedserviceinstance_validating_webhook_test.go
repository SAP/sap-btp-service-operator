package v1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Shared service instance Webhook Test", func() {
	var sharedServiceInstance *SharedServiceInstance
	BeforeEach(func() {
		sharedServiceInstance = getSharedServiceInstance()
	})

	Context("Update", func() {
		When("Spec changed", func() {
			It("should fail", func() {
				newSharedServiceInstance := getSharedServiceInstance()
				newSharedServiceInstance.Spec.ServiceInstanceName = "blabla"
				err := newSharedServiceInstance.ValidateUpdate(sharedServiceInstance)
				Expect(err).To(HaveOccurred())
			})
		})

		When("Spec did not changed", func() {
			It("should succeed", func() {
				err := sharedServiceInstance.ValidateUpdate(sharedServiceInstance)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
