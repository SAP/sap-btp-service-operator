package utils

import (
	"fmt"
	"strings"

	"github.com/lithammer/dedent"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Secret template", func() {

	Context("With valid secretTemplate, but missing keys (credentials are nil)", func() {

		It("should fail", func() {
			nonexistingKey := "nonexistingKey"
			secretTemplate := fmt.Sprintf(
				dedent.Dedent(`
						apiVersion: v1
						kind: Secret
						stringData:
						  foo: {{ .%s }}
					`),
				nonexistingKey,
			)

			secret, err := CreateSecretFromTemplate("", secretTemplate, "missingkey=error", nil)

			Expect(err).Should(MatchError(ContainSubstring("map has no entry for key \"%s\"", nonexistingKey)))
			Expect(secret).Should(BeNil())
		})
	})

	Context("With unknown field", func() {

		It("should succeed and invalid key provided in the secret is ignored", func() {
			expectedSecret := &corev1.Secret{
				TypeMeta: v1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
			}
			secretTemplate := dedent.Dedent(`
					apiVersion: v1
					kind: Secret
					unknownField: foo
				`)

			secret, err := CreateSecretFromTemplate("", secretTemplate, "missingkey=error", nil)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(secret).Should(Equal(expectedSecret))
		})
	})

	Context("With wrong kind", func() {

		It("should fail", func() {
			secretTemplate := dedent.Dedent(`
					apiVersion: v1
					kind: Pod
				`)

			secret, err := CreateSecretFromTemplate("", secretTemplate, "missingkey=error", nil)

			Expect(err).Should(MatchError(
				SatisfyAll(
					ContainSubstring("generated secret manifest has unexpected type"),
					ContainSubstring("Pod"),
				),
			))
			Expect(secret).Should(BeNil())
		})
	})

	Context("With sprig functions", func() {

		It("should fail if forbidden sprig func is used in the template", func() {
			secretTemplate := dedent.Dedent(`
					apiVersion: v1
					kind: Secret
					stringData:
					  foo: {{ .param1 | env }}
				`)

			secret, err := CreateSecretFromTemplate("", secretTemplate, "missingkey=error", nil)

			Expect(err).Should(MatchError(ContainSubstring("function \"env\" not defined")))
			Expect(secret).To(BeNil())
		})

		Describe("limited template output size", func() {

			It("should succeed if template output is too big", func() {
				secretTemplate := dedent.Dedent(`
						apiVersion: v1
						kind: Secret
						stringData:
						  foo: x
					`)
				secretTemplate += strings.Repeat("#", int(templateOutputMaxBytes)-len(secretTemplate))
				Expect(len(secretTemplate)).To(Equal(int(templateOutputMaxBytes)))

				secret, err := CreateSecretFromTemplate("", secretTemplate, "missingkey=error", nil)

				Expect(err).ShouldNot(HaveOccurred())
				Expect(secret).NotTo(BeNil())
			})

			It("should fail if template output is too big", func() {
				secretTemplate := strings.Repeat("a", int(templateOutputMaxBytes)+1)

				secret, err := CreateSecretFromTemplate("", secretTemplate, "missingkey=error", nil)

				Expect(err).Should(MatchError(ContainSubstring("the size of the generated secret manifest exceeds the limit")))
				Expect(secret).To(BeNil())
			})
		})
	})
})
