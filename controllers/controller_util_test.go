package controllers

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Controller Util", func() {

	Context("normalize credentials", func() {
		var credentialsJSON json.RawMessage

		BeforeEach(func() {
			credentialsJSON = []byte(`{"keyStr":"val", "keyBool":true,"keyNum":8,"keyJSON":{"a":"b"}}`)
		})

		It("should normalize correctly", func() {
			res, metadata, err := normalizeCredentials(credentialsJSON)
			str := SecretMetadataProperty{
				Name:   "keyStr",
				Format: string(TEXT),
			}
			boolean := SecretMetadataProperty{
				Name:   "keyBool",
				Format: string(JSON),
			}
			num := SecretMetadataProperty{
				Name:   "keyNum",
				Format: string(JSON),
			}
			json := SecretMetadataProperty{
				Name:   "keyJSON",
				Format: string(JSON),
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(len(res)).To(Equal(4))
			Expect(metadata).To(ContainElements(str, boolean, num, json))
		})

	})
})
