package utils

import (
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/internal/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SM Utils", func() {
	var (
		secret          *corev1.Secret
		tlsSecret       *corev1.Secret
		serviceInstance *v1.ServiceInstance
	)

	BeforeEach(func() {
		InitializeSecretsClient(k8sClient, nil, config.Config{
			ManagementNamespace: managementNamespace,
			ReleaseNamespace:    managementNamespace,
		})
		serviceInstance = &v1.ServiceInstance{Spec: v1.ServiceInstanceSpec{}, Status: v1.ServiceInstanceStatus{}}
		serviceInstance.Name = "test-instance"
		serviceInstance.Namespace = testNamespace
		serviceInstance.Spec = v1.ServiceInstanceSpec{
			ServiceOfferingName: "test",
			ServicePlanName:     "test",
		}
		Expect(k8sClient.Create(ctx, serviceInstance)).To(Succeed())
		Expect(InitConditions(ctx, k8sClient, serviceInstance)).To(Succeed())
	})

	AfterEach(func() {
		if serviceInstance != nil {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, serviceInstance))).To(Succeed())
		}
	})

	Context("GetSMClient", func() {

		AfterEach(func() {
			if secret != nil {
				err := k8sClient.Delete(ctx, secret)
				Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
			}

			if tlsSecret != nil {
				err := k8sClient.Delete(ctx, tlsSecret)
				Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
			}
		})

		Context("SAPBTPOperatorSecret", func() {
			When("secret is valid", func() {
				When("secret contains clientSecret", func() {
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      SAPBTPOperatorSecretName,
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"clientid":     []byte("12345"),
								"clientsecret": []byte("client-secret"),
								"sm_url":       []byte("https://some.url"),
								"tokenurl":     []byte("https://token.url"),
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
					})
					It("should succeed", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).ToNot(HaveOccurred())
						Expect(client).ToNot(BeNil())
					})
				})
				When("secret not contains clientSecret but contains tls data", func() {
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      SAPBTPOperatorSecretName,
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"clientid":     []byte("12345"),
								"clientsecret": []byte(""),
								"sm_url":       []byte("https://some.url"),
								"tokenurl":     []byte("https://token.url"),
								"tls.key":      []byte(tlskey),
								"tls.crt":      []byte(tlscrt),
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
					})
					It("should succeed", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).ToNot(HaveOccurred())
						Expect(client).ToNot(BeNil())
					})
				})
			})
			When("secret is missing client secret and there is no tls secret", func() {
				BeforeEach(func() {
					secret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      SAPBTPOperatorSecretName,
							Namespace: managementNamespace,
						},
						Data: map[string][]byte{
							"clientid":     []byte("12345"),
							"clientsecret": []byte(""),
							"sm_url":       []byte("https://some.url"),
							"tokenurl":     []byte("https://token.url"),
						},
					}
					Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				})
				It("should return error", func() {
					client, err := GetSMClient(ctx, serviceInstance)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
					Expect(client).To(BeNil())
				})
			})
			When("secret is missing token url", func() {
				BeforeEach(func() {
					secret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      SAPBTPOperatorSecretName,
							Namespace: managementNamespace,
						},
						Data: map[string][]byte{
							"clientid":     []byte("12345"),
							"clientsecret": []byte("clientsecret"),
							"sm_url":       []byte("https://some.url"),
							"tokenurl":     []byte(""),
						},
					}
					Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				})
				It("should return error", func() {
					client, err := GetSMClient(ctx, serviceInstance)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
					Expect(client).To(BeNil())
				})
			})
			When("secret is missing sm url", func() {
				BeforeEach(func() {
					secret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      SAPBTPOperatorSecretName,
							Namespace: managementNamespace,
						},
						Data: map[string][]byte{
							"clientid":     []byte("12345"),
							"clientsecret": []byte("clientsecret"),
							"tokenurl":     []byte("http://tokenurl"),
						},
					}
					Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				})
				It("should return error", func() {
					client, err := GetSMClient(ctx, serviceInstance)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
					Expect(client).To(BeNil())
				})
			})
		})

		Context("SAPBTPOperatorTLSSecret", func() {
			BeforeEach(func() {
				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SAPBTPOperatorSecretName,
						Namespace: managementNamespace,
					},
					Data: map[string][]byte{
						"clientid": []byte("12345"),
						"sm_url":   []byte("https://some.url"),
						"tokenurl": []byte("https://token.url"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			})
			When("valid", func() {
				BeforeEach(func() {
					tlsSecret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      SAPBTPOperatorTLSSecretName,
							Namespace: managementNamespace,
						},
						Data: map[string][]byte{
							"tls.key": []byte(tlskey),
							"tls.crt": []byte(tlscrt),
						},
					}
					Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())
				})
				It("should succeed", func() {
					client, err := GetSMClient(ctx, serviceInstance)
					Expect(err).ToNot(HaveOccurred()) //tls: failed to find any PEM data in key input
					Expect(client).ToNot(BeNil())
				})
			})
			When("tls secret not found", func() {
				It("should return error", func() {
					client, err := GetSMClient(ctx, serviceInstance)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
					Expect(client).To(BeNil())
				})
			})
			When("tls secret is missing required values", func() {
				BeforeEach(func() {
					tlsSecret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      SAPBTPOperatorTLSSecretName,
							Namespace: managementNamespace,
						},
						Data: map[string][]byte{
							"tls.key": []byte("12345key"),
						},
					}
					Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())
				})
				It("should return error", func() {
					client, err := GetSMClient(ctx, serviceInstance)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
					Expect(client).To(BeNil())
				})
			})
		})

		Context("btpAccessSecret", func() {
			BeforeEach(func() {
				serviceInstance.Spec.BTPAccessCredentialsSecret = "my-btp-access-secret"
			})
			AfterEach(func() {
				serviceInstance.Spec.BTPAccessCredentialsSecret = ""
			})
			Context("client credentials", func() {
				When("secret is valid", func() {
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "my-btp-access-secret",
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"clientid":     []byte("12345"),
								"clientsecret": []byte("client-secret"),
								"sm_url":       []byte("https://some.url"),
								"tokenurl":     []byte("https://token.url"),
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
						serviceInstance.Spec.BTPAccessCredentialsSecret = "my-btp-access-secret"
					})
					AfterEach(func() {
						serviceInstance.Spec.BTPAccessCredentialsSecret = ""
					})

					It("should succeed", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).ToNot(HaveOccurred())
						Expect(client).ToNot(BeNil())
					})
				})

				When("secret is missing client secret and there is no tls secret", func() {
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "my-btp-access-secret",
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"clientid":     []byte("12345"),
								"clientsecret": []byte(""),
								"sm_url":       []byte("https://some.url"),
								"tokenurl":     []byte("https://token.url"),
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
					})
					It("should return error", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
						Expect(client).To(BeNil())
					})
				})
				When("secret is missing token url", func() {
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "my-btp-access-secret",
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"clientid":     []byte("12345"),
								"clientsecret": []byte("clientsecret"),
								"sm_url":       []byte("https://some.url"),
								"tokenurl":     []byte(""),
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
					})
					It("should return error", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
						Expect(client).To(BeNil())
					})
				})
				When("secret is missing sm url", func() {
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "my-btp-access-secret",
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"clientid":     []byte("12345"),
								"clientsecret": []byte("clientsecret"),
								"tokenurl":     []byte("http://tokenurl"),
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
					})
					It("should return error", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
						Expect(client).To(BeNil())
					})
				})
			})

			Context("tls credentials", func() {
				When("secret is valid", func() {
					BeforeEach(func() {
						secret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "my-btp-access-secret",
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"clientid": []byte("12345"),
								"sm_url":   []byte("https://some.url"),
								"tokenurl": []byte("https://token.url"),
								"tls.key":  []byte(tlskey),
								"tls.crt":  []byte(tlscrt),
							},
						}
						Expect(k8sClient.Create(ctx, secret)).To(Succeed())
					})
					It("should succeed", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).ToNot(HaveOccurred())
						Expect(client).ToNot(BeNil())
					})
				})

				When("tls secret is missing required values", func() {
					BeforeEach(func() {
						tlsSecret = &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "my-btp-access-secret",
								Namespace: managementNamespace,
							},
							Data: map[string][]byte{
								"tls.key": []byte("12345key"),
							},
						}
						Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())
					})
					It("should return error", func() {
						client, err := GetSMClient(ctx, serviceInstance)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
						Expect(client).To(BeNil())
					})
				})
			})
		})
	})
})

// fake TLS certificate and key for testing only — not used in production
const tlscrt = `-----BEGIN CERTIFICATE-----
MIIDFTCCAf2gAwIBAgIUBfv6cKhSA0RFTKSrg2BpHjGbdacwDQYJKoZIhvcNAQEL
BQAwGTEXMBUGA1UEAwwOZmFrZS1sb2NhbGhvc3QwIBcNMjYxMTE5MTEzMTE2WhgP
MjA1NDA0MDUxMTMxMTZaMBkxFzAVBgNVBAMMDmZha2UtbG9jYWxob3N0MIIBIjAN
BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArvlpqA53uJOQWxbQEOQRnF9dxLqS
M7Za5eMx7/094w5/DByegUcNDGJYyxQL5epe2X4WnvYoFTW5rEuRsjtrgl8go1gN
Z43/GgTKRD+dZuEQXOTB2sk89SasUUaT2cSqVCdqCFXLiTS5FYTgU12gIvzELM1a
a1p6hHOhrcJX16zTPlPYovnxDeb1i98cCzV4Mk/l/qWEjSDf9OqBKLb1Ec6KGJYz
NgklInLSt1ScuQd4kMzkgkCWT185cMtLk9QXK5KGywHsFVRwsD1fyqUfkpLXR8xq
d/6yDmm5eXQUG+FufaFFStmjv9r3PhXa94jFZtVrmCi802ulA3LHmJIu6wIDAQAB
o1MwUTAdBgNVHQ4EFgQUxEiOvOo4QKO8Qb+pSEgpdZEbn5kwHwYDVR0jBBgwFoAU
xEiOvOo4QKO8Qb+pSEgpdZEbn5kwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0B
AQsFAAOCAQEAaeQBJnXHKF5n/XrVdEs5V3AyB0KqFoZSy1lq2kQsmxpeOKArDE44
bXz8/PhVsyGpKcgXaZPEpin5YJJCsr9hI7LHUUB5rHLiSBHtOGwvZylVG6wG3wci
M7lVvaQY9w8u91C7/aa751fKL7bYQZ6f9naNhUAZ9od0u/BL19svJT2FYrpqjyQU
DpoVe7goDDRsjFhsViTuU0Wq89Wc9YtTrpFKzaUx5G+jUsSCjtMcSBcCZU46ZHLD
ELJmTUMZ0++8InXmRnmYCNijmo7rS59F8PSFfqBZdK+B9rRa7kwQzIp99p1tBg57
4NYJa5fYkwa0AhpdOXt7BB7E3DXM26DpbA==
-----END CERTIFICATE-----`

const tlskey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCu+WmoDne4k5Bb
FtAQ5BGcX13EupIztlrl4zHv/T3jDn8MHJ6BRw0MYljLFAvl6l7Zfhae9igVNbms
S5GyO2uCXyCjWA1njf8aBMpEP51m4RBc5MHayTz1JqxRRpPZxKpUJ2oIVcuJNLkV
hOBTXaAi/MQszVprWnqEc6GtwlfXrNM+U9ii+fEN5vWL3xwLNXgyT+X+pYSNIN/0
6oEotvURzooYljM2CSUictK3VJy5B3iQzOSCQJZPXzlwy0uT1BcrkobLAewVVHCw
PV/KpR+SktdHzGp3/rIOabl5dBQb4W59oUVK2aO/2vc+Fdr3iMVm1WuYKLzTa6UD
cseYki7rAgMBAAECggEAF9xJLTKz3sa4s8M7omIHiMR0fARZNZYTdS/zOl/VVaY/
av5oGwb+X4b1xtO3/6VjQ9Nu8ERvGN1XsYgomR6DPacNQ1kZktqJRBoNHxtxsnrU
iWZkjRpOJrwcWYT/Wn34IDuzuOlCNTasWqPhF104Hi1L7zQdtXAELxW5oYFUC0P6
/TeVMhWkwMbaQhE6yXMP1eVmLj9G42Kz5U1hWnP6xZffmWz0YIK0un0WkOuT9ui2
c5cvzGcKkkG1OBNOeWBya7U1vTxf6MNBXjdKOrihToi8KJQIQMGPDQa0E+v0Yydv
IdYJdcQfjsyVjY/byN/LU3wPp3jGVdRTojhQh9Pt8QKBgQDtODD2/M1NM0Q9AtoU
gByN2Wt0nirti7B8i1XH2DrLfmIGadnZsHrOHo+eV1iUT1IlTKOgcXCJIk6dWOHM
37hQh+7XgM1quufSQngj8+RIC94cPxR4IHgdWm27OVmR1utWN+EHBjOLX7XAGqfy
XdNpiXTNJ/v29NvTZomNXmFFQwKBgQC8060U4Day929jWPB+xdo7aeaWzB0PaPk+
8n72rTpUcONirMz4LD0YkEgehMKwX33FuQsBcwMgTWWkm29HoNyBSQvEiBqFKLh3
i0jYNh3LQu5JgU7wZm0tpn/mdQnbd3etKUXkYsuVCmgtXcRvYfLh6kkC8CX2ARax
1HvW6UGBOQKBgQDsoSbG2bp5jfA95ABifsD9BqrX84bi2ns8YUk6Romvv+xMdX5o
VlDKF1YwHvUvTEinCGIYg9A8G0rMToM/eNNFEbPAqK1AEB17ce6h/oHzFFOFXa8v
fUQUE8ZuAnFEt1yd7rO77AWyu9BzRqPNcqaXGhS3KXGAERTPNfN0X3csPQKBgDaa
4AqRbEAE3elU5vdCMAHOdihVjnHOYYWaiqy7G4sfuiHm5r89y802rCRnMAXhF3X+
NPiOUfEdcK9RY7TLB/k7LnP/B+xKAk5PsRkVTThiepY3FqTeeeQhqgry/Y7B1ZH1
xYhuKYFIKSg1XYD4Skyae9Klr9ScK/vw/n7/pTnBAoGAFxQZMvfqWT7LarLm8Cf1
0pZoMUfgMKWl2shvl1HnPR3gOdAlIDfCE2QydcG+C64+SzZtd8+9auZA7XL/lf5N
RTISzOGZR6Dcpe8US4/P6U3A6oE350xNc3gKIjqumUQ57YkaGtPZbrM8kfcq15J2
qs/jqUJUAtEF6j0Gw6ZirKw=
-----END PRIVATE KEY-----`
