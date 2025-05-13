package webhooks

import (
	"context"
	"encoding/json"

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

var _ = Describe("ServiceInstanceDefaulter", func() {
	var (
		defaulter *ServiceInstanceDefaulter
		decoder   admission.Decoder
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(servicesv1.AddToScheme(scheme)).To(Succeed())
		decoder = admission.NewDecoder(scheme)
		defaulter = &ServiceInstanceDefaulter{
			Decoder: decoder,
		}
	})

	Describe("On Create Operation", func() {
		It("should default ExternalName and set UserInfo", func() {
			instance := &servicesv1.ServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name: "create-instance",
				},
				Spec: servicesv1.ServiceInstanceSpec{
					ExternalName: "",
				},
			}

			origRaw, err := json.Marshal(instance)
			Expect(err).NotTo(HaveOccurred())

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Create,
					Object: runtime.RawExtension{
						Raw: origRaw,
					},
					UserInfo: authv1.UserInfo{
						Username: "create-user",
						UID:      "create-uid",
						Groups:   []string{"groupA", "groupB"},
						Extra:    map[string]authv1.ExtraValue{"key": {"value1"}},
					},
				},
			}

			resp := defaulter.Handle(context.Background(), req)
			Expect(resp.Result).To(BeNil())
			Expect(resp.Patches).NotTo(BeEmpty())

			patchBytes, err := json.Marshal(resp.Patches)
			Expect(err).NotTo(HaveOccurred())
			patch, err := jsonpatch.DecodePatch(patchBytes)
			Expect(err).NotTo(HaveOccurred())

			mutatedRaw, err := patch.Apply(origRaw)
			Expect(err).NotTo(HaveOccurred())

			var mutatedInstance servicesv1.ServiceInstance
			err = json.Unmarshal(mutatedRaw, &mutatedInstance)
			Expect(err).NotTo(HaveOccurred())

			Expect(mutatedInstance.Spec.ExternalName).To(Equal(instance.Name))
			Expect(mutatedInstance.Spec.UserInfo).NotTo(BeNil())
			Expect(mutatedInstance.Spec.UserInfo.Username).To(Equal("create-user"))
			Expect(mutatedInstance.Spec.UserInfo.UID).To(Equal("create-uid"))
			Expect(mutatedInstance.Spec.UserInfo.Groups).To(Equal([]string{"groupA", "groupB"}))
			Expect(mutatedInstance.Spec.UserInfo.Extra).To(Equal(map[string]authv1.ExtraValue{"key": {"value1"}}))
		})
	})

	Describe("On Update Operation", func() {

		Context("Valid update", func() {
			It("should preserve or update UserInfo when not explicitly modified", func() {
				oldInstance := &servicesv1.ServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name: "update-instance",
					},
					Spec: servicesv1.ServiceInstanceSpec{
						ExternalName: "old-ext",
						UserInfo: &authv1.UserInfo{
							Username: "old-user",
							UID:      "old-uid",
							Groups:   []string{"old-group"},
							Extra:    map[string]authv1.ExtraValue{"old": {"ovalue"}},
						},
					},
				}

				newInstance := &servicesv1.ServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name: "update-instance",
					},
					Spec: servicesv1.ServiceInstanceSpec{
						ExternalName: "",
						// UserInfo is nil, so the webhook should use the request's UserInfo.
					},
				}

				newRaw, err := json.Marshal(newInstance)
				Expect(err).NotTo(HaveOccurred())
				oldRaw, err := json.Marshal(oldInstance)
				Expect(err).NotTo(HaveOccurred())

				req := admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Update,
						Object: runtime.RawExtension{
							Raw: newRaw,
						},
						OldObject: runtime.RawExtension{
							Raw: oldRaw,
						},
						UserInfo: authv1.UserInfo{
							Username: "update-user",
							UID:      "update-uid",
							Groups:   []string{"groupX"},
							Extra:    map[string]authv1.ExtraValue{"key": {"newValue"}},
						},
					},
				}

				resp := defaulter.Handle(context.Background(), req)
				Expect(resp.Result).To(BeNil())
				Expect(resp.Patches).NotTo(BeEmpty())

				patchBytes, err := json.Marshal(resp.Patches)
				Expect(err).NotTo(HaveOccurred())
				patch, err := jsonpatch.DecodePatch(patchBytes)
				Expect(err).NotTo(HaveOccurred())

				mutatedRaw, err := patch.Apply(newRaw)
				Expect(err).NotTo(HaveOccurred())

				var mutatedInstance servicesv1.ServiceInstance
				err = json.Unmarshal(mutatedRaw, &mutatedInstance)
				Expect(err).NotTo(HaveOccurred())

				Expect(mutatedInstance.Spec.ExternalName).To(Equal(newInstance.Name))
				Expect(mutatedInstance.Spec.UserInfo).NotTo(BeNil())
				Expect(mutatedInstance.Spec.UserInfo.Username).To(Equal("update-user"))
				Expect(mutatedInstance.Spec.UserInfo.UID).To(Equal("update-uid"))
				Expect(mutatedInstance.Spec.UserInfo.Groups).To(Equal([]string{"groupX"}))
				Expect(mutatedInstance.Spec.UserInfo.Extra).To(Equal(map[string]authv1.ExtraValue{"key": {"newValue"}}))
			})
		})

		Context("Invalid update", func() {
			It("should error if Spec.UserInfo is explicitly modified", func() {
				oldInstance := &servicesv1.ServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name: "update-instance",
					},
					Spec: servicesv1.ServiceInstanceSpec{
						ExternalName: "existing-ext",
						UserInfo: &authv1.UserInfo{
							Username: "old-user",
							UID:      "old-uid",
							Groups:   []string{"old-group"},
							Extra:    map[string]authv1.ExtraValue{"old": {"ovalue"}},
						},
					},
				}

				newInstance := &servicesv1.ServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name: "update-instance",
					},
					Spec: servicesv1.ServiceInstanceSpec{
						ExternalName: "existing-ext",
						UserInfo: &authv1.UserInfo{
							Username: "illegal-change",
							UID:      "illegal-uid",
							Groups:   []string{"new-group"},
							Extra:    map[string]authv1.ExtraValue{"new": {"value"}},
						},
					},
				}

				newRaw, err := json.Marshal(newInstance)
				Expect(err).NotTo(HaveOccurred())
				oldRaw, err := json.Marshal(oldInstance)
				Expect(err).NotTo(HaveOccurred())

				req := admission.Request{
					AdmissionRequest: admissionv1.AdmissionRequest{
						Operation: admissionv1.Update,
						Object: runtime.RawExtension{
							Raw: newRaw,
						},
						OldObject: runtime.RawExtension{
							Raw: oldRaw,
						},
						UserInfo: authv1.UserInfo{
							Username: "update-user",
							UID:      "update-uid",
						},
					},
				}

				resp := defaulter.Handle(context.Background(), req)
				Expect(resp.Result).NotTo(BeNil())
				Expect(resp.Result.Message).To(ContainSubstring("modifying spec.userInfo is not allowed"))
			})
		})
	})
})
