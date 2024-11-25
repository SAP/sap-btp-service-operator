package utils

import (
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Parameters", func() {
	Describe("BuildSMRequestParameters", func() {
		It("handles empty parameters", func() {
			var parametersFrom []v1.ParametersFromSource
			parameters := (*runtime.RawExtension)(nil)

			rawParam, secrets, err := BuildSMRequestParameters("", parameters, parametersFrom)

			Expect(err).To(BeNil())
			Expect(rawParam).To(BeNil())
			Expect(len(secrets)).To(BeZero())
		})
		It("handles parameters from source", func() {
			var parametersFrom []v1.ParametersFromSource
			parameters := &runtime.RawExtension{
				Raw: []byte(`{"key":"value"}`),
			}

			rawParam, secrets, err := BuildSMRequestParameters("", parameters, parametersFrom)

			Expect(err).To(BeNil())
			Expect(rawParam).To(Equal([]byte(`{"key":"value"}`)))
			Expect(len(secrets)).To(BeZero())
		})
	})
})
