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

			params, rawParam, err := BuildSMRequestParameters(nil, "", parametersFrom, parameters)

			Expect(err).To(BeNil())
			Expect(params).To(BeNil())
			Expect(rawParam).To(BeNil())
		})
		It("handles parameters from source", func() {
			var parametersFrom []v1.ParametersFromSource
			parameters := &runtime.RawExtension{
				Raw: []byte(`{"key":"value"}`),
			}

			params, rawParam, err := BuildSMRequestParameters(nil, "", parametersFrom, parameters)

			Expect(err).To(BeNil())
			Expect(params).To(Equal(map[string]interface{}{"key": "value"}))
			Expect(rawParam).To(Equal([]byte(`{"key":"value"}`)))
		})
	})
})
