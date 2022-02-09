package v1alpha1

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Service Binding Webhook Test", func() {
	var binding *ServiceBinding
	BeforeEach(func() {
		binding = getBinding()
	})

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
					It("should succeed", func() {
						newBinding.Spec.ServiceInstanceName = "new-service-instance"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("External name changed", func() {
					It("should succeed", func() {
						newBinding.Spec.ExternalName = "new-external-instance"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("Parameters were changed", func() {
					It("should succeed", func() {
						newBinding.Spec.Parameters = &runtime.RawExtension{
							Raw: []byte("params"),
						}
						err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("ParametersFrom were changed", func() {
					It("should succeed", func() {
						newBinding.Spec.ParametersFrom[0].SecretKeyRef.Name = "newName"
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

			When("CredConfig changed", func() {
				It("should succeed", func() {
					newBinding.Spec.CredRotationConfig = &CredentialsRotationConfiguration{
						Enabled: true,
					}
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should fail when keepFor > rotationInterval", func() {
					newBinding.Spec.CredRotationConfig = &CredentialsRotationConfiguration{
						Enabled:          true,
						KeepFor:          time.Hour,
						RotationInterval: time.Minute,
					}
					err := newBinding.ValidateUpdate(binding)
					Expect(err).To(HaveOccurred())
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

				When("secret name changed", func() {
					It("should fail", func() {
						newBinding.Spec.SecretName = "newsecret"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("SecretKey name changed", func() {
					It("should fail", func() {
						secretKey := "secret-key"
						newBinding.Spec.SecretKey = &secretKey
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("SecretRootKey name changed", func() {
					It("should fail", func() {
						secretRootKey := "root"
						newBinding.Spec.SecretRootKey = &secretRootKey
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

				When("ParametersFrom were changed", func() {
					It("should fail on changed name", func() {
						newBinding.Spec.ParametersFrom[0].SecretKeyRef.Name = "newName"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})

					It("should fail on changed key", func() {
						newBinding.Spec.ParametersFrom[0].SecretKeyRef.Key = "newName"
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})

					It("should fail on nil array", func() {
						newBinding.Spec.ParametersFrom = nil
						err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})

					It("should fail on changed array", func() {
						p := ParametersFromSource{}
						newBinding.Spec.ParametersFrom[0] = p
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

			When("CredConfig changed", func() {
				It("should succeed", func() {
					newBinding.Spec.CredRotationConfig = &CredentialsRotationConfiguration{
						Enabled: true,
					}
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should fail when keepFor > rotationInterval", func() {
					newBinding.Spec.CredRotationConfig = &CredentialsRotationConfiguration{
						Enabled:          true,
						KeepFor:          time.Hour,
						RotationInterval: time.Minute,
					}
					err := newBinding.ValidateUpdate(binding)
					Expect(err).To(HaveOccurred())
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
