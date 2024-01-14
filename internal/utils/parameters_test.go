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
			parametersFrom := []v1.ParametersFromSource{}
			parameters := (*runtime.RawExtension)(nil)

			params, rawParam, err := BuildSMRequestParameters(nil, "", parametersFrom, parameters)

			Expect(err).To(BeNil())
			Expect(params).To(BeNil())
			Expect(rawParam).To(BeNil())
		})
	})
})
