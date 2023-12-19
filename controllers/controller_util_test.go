package controllers

import (
	"encoding/json"
	"github.com/SAP/sap-btp-service-operator/api"
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"time"
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

	Context("shouldIgnoreNonTransient", func() {
		var (
			instance *v1.ServiceInstance
			logger   = logf.Log.WithName("test-logger")
		)

		BeforeEach(func() {
			instance = &v1.ServiceInstance{}
		})

		It("should return false if no ignore annotation", func() {
			instance.SetAnnotations(nil)
			Expect(shouldIgnoreNonTransient(logger, instance, time.Hour)).To(BeFalse())
		})

		It("should return false if time exceeded", func() {
			annotation := map[string]string{
				api.IgnoreNonTransientErrorAnnotation:          "true",
				api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Truncate(48 * time.Hour).Format(time.RFC3339),
			}
			instance.SetAnnotations(annotation)
			Expect(shouldIgnoreNonTransient(logger, instance, time.Hour)).To(BeFalse())
		})

		It("should return true if time not exceeded", func() {
			annotation := map[string]string{
				api.IgnoreNonTransientErrorAnnotation:          "true",
				api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Format(time.RFC3339),
			}
			instance.SetAnnotations(annotation)
			Expect(shouldIgnoreNonTransient(logger, instance, time.Hour)).To(BeTrue())
		})
	})
})
