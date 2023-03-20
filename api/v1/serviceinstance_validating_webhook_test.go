package v1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
)

var _ = Describe("Service Instance Webhook Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
	})

	Context("Validator", func() {
		Context("Validate un shared instance", func() {
			var newInstance *ServiceInstance
			BeforeEach(func() {
				newInstance = getNonSharedInstance()
				newInstance.Status.InstanceID = "1234"
			})

			When("Spec changed", func() {
				It("should fail", func() {
					newInstance.Spec.Shared = pointer.BoolPtr(true)
					newInstance.Spec.ExternalName = "blabla"
					err := newInstance.ValidateUpdate(instance)
					Expect(err).To(HaveOccurred())
				})
			})

			When("Spec did not change", func() {
				It("should pass", func() {
					newInstance.Spec.Shared = pointer.BoolPtr(true)
					err := newInstance.ValidateUpdate(instance)
					Expect(err).To(Not(HaveOccurred()))
				})
			})
		})

		Context("Validate shared instance", func() {
			var newInstance *ServiceInstance
			BeforeEach(func() {
				newInstance = getSharedInstance()
				newInstance.Status.InstanceID = "1234"
			})

			When("Spec changed", func() {
				It("should fail", func() {
					newInstance.Spec.Shared = pointer.BoolPtr(false)
					newInstance.Spec.ExternalName = "blabla"
					err := newInstance.ValidateUpdate(instance)
					Expect(err).To(HaveOccurred())
				})
			})

			When("Spec did not change", func() {
				It("should pass", func() {
					newInstance.Spec.Shared = pointer.BoolPtr(false)
					err := newInstance.ValidateUpdate(instance)
					Expect(err).To(Not(HaveOccurred()))
				})
			})
		})
	})
})
