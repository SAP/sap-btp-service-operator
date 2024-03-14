package v1

import (
	"github.com/SAP/sap-btp-service-operator/api/common"
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
				_, err := binding.ValidateCreate()
				Expect(err).ToNot(HaveOccurred())
			})
			It("should succeed if secretTemplate can be parsed", func() {
				binding.Spec.SecretTemplate = dedent.Dedent(
					`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: "a-new-plan-name"
  annotations:
    instance_name: "a-new-instance-name"
stringData:
  newKey2: {{ .credentials.secret_key }}`)
				_, err := binding.ValidateCreate()

				Expect(err).ToNot(HaveOccurred())
			})
			It("should fail if can't secretTemplate can be parsed", func() {
				//write test for secretTemplateError
				binding.Spec.SecretTemplate = "{{"
				_, err := binding.ValidateCreate()
				Expect(err).To(HaveOccurred())
				errMsg := err.Error()
				Expect(errMsg).To(ContainSubstring("unclosed action"))
			})
			It("should fail if can't secretTemplate have invalid function", func() {
				//write test for secretTemplateError
				binding.Spec.SecretTemplate = dedent.Dedent(`
				                                       apiVersion: v1
				                                       kind: Secret
				                                       stringData:
				                                         secretKey: {{ .secretValue | quote }}`)
				_, err := binding.ValidateCreate()
				Expect(err).To(HaveOccurred())
				errMsg := err.Error()
				Expect(errMsg).To(ContainSubstring(" function \"quote\" not defined"))
			})
			It("should fail if template contains metadata.name", func() {
				//write test for secretTemplateError
				binding.Spec.SecretTemplate = dedent.Dedent(
					`apiVersion: v1
kind: Secret
metadata:
  name: "a-new-secret"
  labels:
    instance_plan: "a-new-plan-name"
  annotations:
    instance_name: "a-new-instance-name"
stringData:
  newKey2: {{ .credentials.secret_key }}`)

				_, err := binding.ValidateCreate()
				Expect(err).To(HaveOccurred())
				errMsg := err.Error()
				Expect(errMsg).To(ContainSubstring("the Secret template is invalid: Secret's metadata field"))
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
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("External name changed", func() {
					It("should succeed", func() {
						newBinding.Spec.ExternalName = "new-external-instance"
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("Parameters were changed", func() {
					It("should succeed", func() {
						newBinding.Spec.Parameters = &runtime.RawExtension{
							Raw: []byte("params"),
						}
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("ParametersFrom were changed", func() {
					It("should succeed", func() {
						newBinding.Spec.ParametersFrom[0].SecretKeyRef.Name = "newName"
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})

				When("SecretTemplate changed", func() {
					It("should succeed", func() {
						modifiedSecretTemplate := dedent.Dedent(`
						apiVersion: v1
						kind: Secret
						stringData:
						  key2: "value2"
					`)
						newBinding.Spec.SecretTemplate = modifiedSecretTemplate
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})

			When("Metadata changed", func() {
				It("should succeed", func() {
					newBinding.Finalizers = append(newBinding.Finalizers, "newFinalizer")
					_, err := newBinding.ValidateUpdate(binding)
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
					_, err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should fail when duration format not valid", func() {
					newBinding.Spec.CredRotationPolicy = &CredentialsRotationPolicy{
						Enabled:           true,
						RotatedBindingTTL: "1x",
						RotationFrequency: "1y",
					}
					_, err := newBinding.ValidateUpdate(binding)
					Expect(err).To(HaveOccurred())
				})

				It("should fail on update with stale label", func() {
					binding.Labels = map[string]string{common.StaleBindingIDLabel: "true"}
					newBinding.Spec.ParametersFrom[0].SecretKeyRef.Name = "newName"
					newBinding.Labels = map[string]string{common.StaleBindingIDLabel: "true"}
					_, err := newBinding.ValidateUpdate(binding)
					Expect(err).To(HaveOccurred())
				})

			})

			When("Status changed", func() {
				It("should succeed", func() {
					newBinding.Status.BindingID = "12345"
					_, err := newBinding.ValidateUpdate(binding)
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
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("External name changed", func() {
					It("should fail", func() {
						newBinding.Spec.ExternalName = "new-external-instance"
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("secret name changed", func() {
					It("should fail", func() {
						newBinding.Spec.SecretName = "newsecret"
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("SecretKey name changed", func() {
					It("should fail", func() {
						secretKey := "secret-key"
						newBinding.Spec.SecretKey = &secretKey
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("SecretRootKey name changed", func() {
					It("should fail", func() {
						secretRootKey := "root"
						newBinding.Spec.SecretRootKey = &secretRootKey
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("Parameters were changed", func() {
					It("should fail", func() {
						newBinding.Spec.Parameters = &runtime.RawExtension{
							Raw: []byte("params"),
						}
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})
				})

				When("ParametersFrom were changed", func() {
					It("should fail on changed name", func() {
						newBinding.Spec.ParametersFrom[0].SecretKeyRef.Name = "newName"
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})

					It("should fail on changed key", func() {
						newBinding.Spec.ParametersFrom[0].SecretKeyRef.Key = "newName"
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})

					It("should fail on nil array", func() {
						newBinding.Spec.ParametersFrom = nil
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})

					It("should fail on changed array", func() {
						p := ParametersFromSource{}
						newBinding.Spec.ParametersFrom[0] = p
						_, err := newBinding.ValidateUpdate(binding)
						Expect(err).To(HaveOccurred())
					})

				})
			})

			When("Metadata changed", func() {
				It("should succeed", func() {
					newBinding.Finalizers = append(newBinding.Finalizers, "newFinalizer")
					_, err := newBinding.ValidateUpdate(binding)
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
					_, err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should fail when duration format not valid", func() {
					newBinding.Spec.CredRotationPolicy = &CredentialsRotationPolicy{
						Enabled:           true,
						RotatedBindingTTL: "1x",
						RotationFrequency: "1y",
					}
					_, err := newBinding.ValidateUpdate(binding)
					Expect(err).To(HaveOccurred())
				})
			})

			When("Status changed", func() {
				It("should succeed", func() {
					newBinding.Status.BindingID = "12345"
					_, err := newBinding.ValidateUpdate(binding)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		Context("Validate delete", func() {
			It("should succeed", func() {
				_, err := binding.ValidateDelete()
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
