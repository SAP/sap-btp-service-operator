package v1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Instance Webhook Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
	})

	FContext("Validator", func() {
		Context("Validate create", func() {
			It("should succeed", func() {
				err := instance.ValidateCreate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Validate un shared instance", func() {
			var newInstance *ServiceInstance
			BeforeEach(func() {
				newInstance = getNonSharedInstance()
				newInstance.Status.InstanceID = "1234"
			})

			When("Spec changed", func() {
				It("should fail", func() {
					newInstance.Spec.Shared = true
					newInstance.Spec.ExternalName = "Hunni"
					err := newInstance.ValidateUpdate(instance)
					Expect(err).To(HaveOccurred())
				})
			})

			When("Spec did not change", func() {
				It("should pass", func() {
					newInstance.Spec.Shared = true
					err := newInstance.ValidateUpdate(instance)
					Expect(err).To(Not(HaveOccurred()))
				})
			})
		})

		Context("Validate delete", func() {
			It("should succeed", func() {
				err := instance.ValidateDelete()
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
