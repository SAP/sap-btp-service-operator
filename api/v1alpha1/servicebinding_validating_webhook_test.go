package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Service Binding Webhook Test", func() {
	var binding *ServiceBinding
	BeforeEach(func() {
		binding = getBinding()
	})
	/*Context("Defaulter", func() {
		When("No external name provided", func() {
			BeforeEach(func() {
				binding.Spec.ExternalName = ""
			})
			It("should add default", func() {
				binding.Default()
				Expect(binding.Spec.ExternalName).To(Equal(binding.Name))
			})
		})

		When("No secret name provided", func() {
			BeforeEach(func() {
				binding.Spec.SecretName = ""
			})
			It("should add default", func() {
				binding.Default()
				Expect(binding.Spec.SecretName).To(Equal(binding.Name))
			})
		})
	})*/

	Context("Validator", func() {
		Context("Validate create", func() {
			It("should succeed", func() {
				err := binding.ValidateCreate()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("Validate update of spec before binding is created (failure recovery)", func() {
			var newBinding *ServiceBinding

			BeforeEach(func() {
				newBinding = getBinding()
			})
			When("Spec changed", func() {
				When("Service instance name changed", func() {
					It("should fail", func() {
						newBinding.Spec.ServiceInstanceName = "new-service-instance"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("External name changed", func() {
					It("should fail", func() {
						newBinding.Spec.ExternalName = "new-external-instance"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("Parameters were changed", func() {
					It("should fail", func() {
						newBinding.Spec.Parameters = &runtime.RawExtension{
							Raw: []byte("params"),
						}
						err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})

			When("Metadata changed", func() {
				It("should succeed", func() {
					newBinding.Finalizers = append(newBinding.Finalizers, "newFinalizer")
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("Status changed", func() {
				It("should succeed", func() {
					newBinding.Status.BindingID = "12345"
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		Context("Validate update of spec after binding is created", func() {
			var newBinding *ServiceBinding
			BeforeEach(func() {
				newBinding = getBinding()
				newBinding.Status.BindingID = "1234"
			})
			When("Spec changed", func() {
				When("Service instance name changed", func() {
					It("should fail", func() {
						newBinding.Spec.ServiceInstanceName = "new-service-instance"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("External name changed", func() {
					It("should fail", func() {
						newBinding.Spec.ExternalName = "new-external-instance"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("Parameters were changed", func() {
					It("should fail", func() {
						newBinding.Spec.Parameters = &runtime.RawExtension{
							Raw: []byte("params"),
						}
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})
			})

			When("Metadata changed", func() {
				It("should succeed", func() {
					newBinding.Finalizers = append(newBinding.Finalizers, "newFinalizer")
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			When("Status changed", func() {
				It("should succeed", func() {
					newBinding.Status.BindingID = "12345"
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		Context("Validate delete", func() {
			It("should succeed", func() {
				err := binding.ValidateDelete()
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
