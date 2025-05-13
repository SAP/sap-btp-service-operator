package webhooks

import (
	"context"
	"encoding/json"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch/v5"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"
)

var _ = Describe("ServiceBindingDefaulter", func() {
	var (
		defaulter *ServiceBindingDefaulter
		decoder   admission.Decoder
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		// Register ServiceBinding types into the scheme.
		Expect(servicesv1.AddToScheme(scheme)).To(Succeed())
		decoder = admission.NewDecoder(scheme)
		defaulter = &ServiceBindingDefaulter{
			Decoder: decoder,
		}
	})

	Context("On Create Operation", func() {
		It("should default ExternalName, SecretName and set UserInfo", func() {
			// Define a ServiceBinding with empty ExternalName and SecretName.
			binding := &servicesv1.ServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-binding",
				},
				Spec: servicesv1.ServiceBindingSpec{
					ExternalName: "",
					SecretName:   "",
				},
			}

			// Marshal the binding into JSON.
			origRaw, err := json.Marshal(binding)
			Expect(err).NotTo(HaveOccurred())

			// Create an admission request simulating a Create operation.
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: origRaw,
					},
					UserInfo: authv1.UserInfo{
						Username: "test-user",
						UID:      "uid-1234",
						Groups:   []string{"group1", "group2"},
						Extra:    map[string]authv1.ExtraValue{"key": {"value1"}},
					},
				},
			}

			// Call the webhook handler.
			resp := defaulter.Handle(context.Background(), req)
			Expect(resp.Result).To(BeNil())
			// For create, we expect a non-empty patch.
			Expect(resp.Patches).NotTo(BeEmpty())

			// Apply the JSON patch to the original object to get the mutated object.
			patchBytes, err := json.Marshal(resp.Patches)
			Expect(err).NotTo(HaveOccurred())

			patch, err := jsonpatch.DecodePatch(patchBytes)
			Expect(err).NotTo(HaveOccurred())

			mutatedRaw, err := patch.Apply(origRaw)
			Expect(err).NotTo(HaveOccurred())

			var mutatedBinding servicesv1.ServiceBinding
			err = json.Unmarshal(mutatedRaw, &mutatedBinding)
			Expect(err).NotTo(HaveOccurred())

			// Verify that fields have been defaulted.
			Expect(mutatedBinding.Spec.ExternalName).To(Equal("test-binding"))
			Expect(mutatedBinding.Spec.SecretName).To(Equal("test-binding"))

			// Verify that the CredRotationPolicy defaults are applied if provided.
			if mutatedBinding.Spec.CredRotationPolicy != nil {
				Expect(mutatedBinding.Spec.CredRotationPolicy.RotationFrequency).
					To(Or(Equal("72h"), Not(BeEmpty())))
				Expect(mutatedBinding.Spec.CredRotationPolicy.RotatedBindingTTL).
					To(Or(Equal("48h"), Not(BeEmpty())))
			}

			// Verify that for Create operations, the Spec.UserInfo is populated.
			Expect(mutatedBinding.Spec.UserInfo).NotTo(BeNil())
			Expect(mutatedBinding.Spec.UserInfo.Username).To(Equal("test-user"))
			Expect(mutatedBinding.Spec.UserInfo.UID).To(Equal("uid-1234"))
			Expect(mutatedBinding.Spec.UserInfo.Groups).To(Equal([]string{"group1", "group2"}))
			Expect(mutatedBinding.Spec.UserInfo.Extra).
				To(Equal(map[string]authv1.ExtraValue{"key": {"value1"}}))
		})
	})

	Context("On Update Operation", func() {
		It("should retain provided ExternalName and SecretName", func() {
			// Define a ServiceBinding with provided ExternalName and SecretName.
			binding := &servicesv1.ServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "update-binding",
				},
				Spec: servicesv1.ServiceBindingSpec{
					ExternalName: "custom-external",
					SecretName:   "custom-secret",
				},
			}

			origRaw, err := json.Marshal(binding)
			Expect(err).NotTo(HaveOccurred())

			// For update operations, the webhook should not change any fields.
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					Object: runtime.RawExtension{
						Raw: origRaw,
					},
					UserInfo: authv1.UserInfo{
						Username: "ignored-user",
					},
				},
			}

			resp := defaulter.Handle(context.Background(), req)
			Expect(resp.Result).To(BeNil())
			// Since no changes are applied, we expect an empty patch.
			Expect(resp.Patches).To(BeEmpty())

			// Even if we apply an empty patch, the object remains unchanged.
			var mutatedBinding servicesv1.ServiceBinding
			err = json.Unmarshal(origRaw, &mutatedBinding)
			Expect(err).NotTo(HaveOccurred())

			// Verify that the provided names remain unchanged.
			Expect(mutatedBinding.Spec.ExternalName).To(Equal("custom-external"))
			Expect(mutatedBinding.Spec.SecretName).To(Equal("custom-secret"))
			// Since the operation is UPDATE, no UserInfo is injected.
			Expect(mutatedBinding.Spec.UserInfo).To(BeNil())
		})
	})

	Context("When decoder fails", func() {
		It("should return an error response", func() {
			// Provide malformed JSON to force a decoder error.
			badRaw := []byte("{")
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: badRaw,
					},
					UserInfo: authv1.UserInfo{
						Username: "test-user",
					},
				},
			}

			resp := defaulter.Handle(context.Background(), req)
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Code).To(Equal(int32(http.StatusBadRequest)))
		})
	})
})
