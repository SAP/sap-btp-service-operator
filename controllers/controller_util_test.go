package controllers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Controller Util Test Test", func() {
	BeforeEach(func() {
	})

	It("should return plain data as is", func() {
		normalized, _ := normalizeCredentials([]byte(`{"key": "value"}`), 1)
		Expect(normalized).To(Equal(map[string][]byte{"key": []byte("value")}))
	})

	It("should create nested JSON structures", func() {
		normalized, _ := normalizeCredentials([]byte(`{"key": {"nested_key": "nested_value"}}`), 1)
		Expect(normalized).To(Equal(map[string][]byte{"key": []byte(`{"nested_key":"nested_value"}`)}))
	})

	It("should flatten nested JSON structures to level 2", func() {
		normalized, _ := normalizeCredentials([]byte(`{"outerkey": {"nested_key": "nested_value", "deep_nesting": { "deep_nested_key": "deep_nested_value" }}}`), 2)
		Expect(normalized).To(Equal(map[string][]byte{"outerkey_nested_key": []byte(`nested_value`), "outerkey_deep_nesting": []byte("{\"deep_nested_key\":\"deep_nested_value\"}")}))
	})

	It("should flatten nested JSON structures to level 3", func() {
		normalized, _ := normalizeCredentials([]byte(`{"outerkey": {"nested_key": "nested_value", "deep_nesting": { "deep_nested_key": "deep_nested_value" }}}`), 3)
		Expect(normalized).To(Equal(map[string][]byte{"outerkey_nested_key": []byte(`nested_value`), "outerkey_deep_nesting_deep_nested_key": []byte(`deep_nested_value`)}))
	})
})
