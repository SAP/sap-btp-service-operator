package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	"github.com/lithammer/dedent"
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

			It("should succeed if secretTemplate can be parsed", func() {
				binding.Spec.SecretTemplate = dedent.Dedent(`
					apiVersion: v1
					kind: Secret
					stringData:
					  secretKey: {{ .secretValue | quote }}`)

				err := binding.ValidateCreate()

				Expect(err).ToNot(HaveOccurred())
			})

			It("should fail if secretTemplate cannot be parsed", func() {
				binding.Spec.SecretTemplate = dedent.Dedent(`
					apiVersion: v1
					kind: Secret
					stringData:
					  secretKey: {{ .secretValue | quote`)

				err := binding.ValidateCreate()

				Expect(err).Should(MatchError(ContainSubstring("spec.secretTemplate is invalid")))
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

				When("SecretTemplate changed", func() {
					It("should succeed", func() {
						modifiedSecretTemplate := `
							apiVersion: v1
							kind: Secret
							stringData:
							  key2: "value2"`
						newBinding.Spec.SecretTemplate = modifiedSecretTemplate
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
					newBinding.Spec.CredRotationPolicy = &CredentialsRotationPolicy{
						Enabled:           true,
						RotatedBindingTTL: "1s",
						RotationFrequency: "1s",
					}
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should fail when duration format not valid", func() {
					newBinding.Spec.CredRotationPolicy = &CredentialsRotationPolicy{
						Enabled:           true,
						RotatedBindingTTL: "1x",
						RotationFrequency: "1y",
					}
					err := newBinding.ValidateUpdate(binding)
					Expect(err).To(HaveOccurred())
				})

				It("should fail on update with stale label", func() {
					newBinding.Spec.ParametersFrom[0].SecretKeyRef.Name = "newName"
					newBinding.Labels = map[string]string{api.StaleBindingIDLabel: "true"}
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

				When("SecretTemplate changed", func() {
					It("should fail", func() {
						modifiedSecretTemplate := `
							apiVersion: v1
							kind: Secret
							stringData:
							  key2: value2`
						newBinding.Spec.SecretTemplate = modifiedSecretTemplate
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
					newBinding.Spec.CredRotationPolicy = &CredentialsRotationPolicy{
						Enabled:           true,
						RotatedBindingTTL: "1s",
						RotationFrequency: "1s",
					}
					err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should fail when duration format not valid", func() {
					newBinding.Spec.CredRotationPolicy = &CredentialsRotationPolicy{
						Enabled:           true,
						RotatedBindingTTL: "1x",
						RotationFrequency: "1y",
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
