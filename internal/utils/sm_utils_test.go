package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("SM Utils", func() {
	var resolver *SecretResolver
	var secret *corev1.Secret
	var tlsSecret *corev1.Secret

	BeforeEach(func() {
		resolver = &SecretResolver{
			ManagementNamespace: managementNamespace,
			ReleaseNamespace:    managementNamespace,
			Log:                 logf.Log.WithName("SecretResolver"),
			Client:              k8sClient,
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
					client, err := GetSMClient(ctx, resolver, testNamespace, "")
					Expect(err).ToNot(HaveOccurred())
					Expect(client).ToNot(BeNil())
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
					client, err := GetSMClient(ctx, resolver, testNamespace, "")
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
					client, err := GetSMClient(ctx, resolver, testNamespace, "")
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
					client, err := GetSMClient(ctx, resolver, testNamespace, "")
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
					client, err := GetSMClient(ctx, resolver, testNamespace, "")
					Expect(err).ToNot(HaveOccurred()) //tls: failed to find any PEM data in key input
					Expect(client).ToNot(BeNil())
				})
			})
			When("tls secret not found", func() {
				It("should return error", func() {
					client, err := GetSMClient(ctx, resolver, testNamespace, "")
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
					client, err := GetSMClient(ctx, resolver, testNamespace, "")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
					Expect(client).To(BeNil())
				})
			})
		})

		Context("btpAccessSecret", func() {
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
					})
					It("should succeed", func() {
						client, err := GetSMClient(ctx, resolver, testNamespace, "my-btp-access-secret")
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
						client, err := GetSMClient(ctx, resolver, testNamespace, "my-btp-access-secret")
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
						client, err := GetSMClient(ctx, resolver, testNamespace, "my-btp-access-secret")
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
						client, err := GetSMClient(ctx, resolver, testNamespace, "my-btp-access-secret")
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
						client, err := GetSMClient(ctx, resolver, testNamespace, "my-btp-access-secret")
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
						client, err := GetSMClient(ctx, resolver, testNamespace, "my-btp-access-secret")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("invalid Service-Manager credentials, contact your cluster administrator"))
						Expect(client).To(BeNil())
					})
				})
			})
		})
	})
})

const tlscrt = `-----BEGIN CERTIFICATE-----
MIIDjDCCAnSgAwIBAgIJAKqmq2MCqIgGMA0GCSqGSIb3DQEBCwUAMCcxCzAJBgNV
BAYTAlVTMRgwFgYDVQQDDA9FeGFtcGxlLVJvb3QtQ0EwIBcNMjIxMDI1MDkzNTI3
WhgPMjA1MDExMDcwOTM1MjdaMGcxCzAJBgNVBAYTAlVTMRIwEAYDVQQIDAlZb3Vy
U3RhdGUxETAPBgNVBAcMCFlvdXJDaXR5MR0wGwYDVQQKDBRFeGFtcGxlLUNlcnRp
ZmljYXRlczESMBAGA1UEAwwJbG9jYWxob3N0MIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEA6pbBoAKsnVVO0e9ihC7AXpMJmW4v8TMEDcuDHVPHn7ZT8aUI
v87yT6Mfy+Qb5/XKohTvnLmQvirf4dnqzDxk/S4//8uu0j5zK7iDqfqPVXlwSGXq
l7uavEnCSRQSp8SbaQaUymEZ7nbjjycfJp/uNLJGFShft4wWyHABAsIYg3FqVjfm
UgacoasTyBzjvogBAsZAd9jpIVUQFvb389IacQKk0p6tJ/r7CWlkscvVV+ToyNTx
0538zBwksEzUnGepEJS9rVHKBVTC7Kz/TltUVxoNIZM7UIrCReJEHkOtpHqGcaHs
6S6FVgId+B4YcoZDoc/RE/XiwOPldSXgWOrnvwIDAQABo3kwdzBBBgNVHSMEOjA4
oSukKTAnMQswCQYDVQQGEwJVUzEYMBYGA1UEAwwPRXhhbXBsZS1Sb290LUNBggkA
v5wjvJ5SVsIwCQYDVR0TBAIwADALBgNVHQ8EBAMCBPAwGgYDVR0RBBMwEYIJbG9j
YWxob3N0hwR/AAABMA0GCSqGSIb3DQEBCwUAA4IBAQCT60QRqid/IDQCZ1x5LVfN
KltSBT+oogZtEM15yL+at0XshiG+UjA7VuLJXRrLcLWya8dzTRombx52v3gFpTGG
YEKxMNXme3KnbVQOWPO1voTiOM8TmJC+7kdUWwv0ghGvudjKTJ51B7kJvph475IZ
Y2SzAPU3ZKeRkNRDMBTl85Ua6NPDq+5dj9NxNhylyhKwP4qf1SocgoB2NVNe9cVU
HQfkmCLS06+y3lrb9C86+SlMmtEouoWymiKZv9pUkSTDUL/Cpk9AdMBWU93aNN8y
DGtUtVWQd2nofkg+l9Yoonsh/QZENSTIL5OA+HPHOlpeZZ2D3vvJXqpuGVUt+A1K
-----END CERTIFICATE-----`

const tlskey = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDqlsGgAqydVU7R
72KELsBekwmZbi/xMwQNy4MdU8eftlPxpQi/zvJPox/L5Bvn9cqiFO+cuZC+Kt/h
2erMPGT9Lj//y67SPnMruIOp+o9VeXBIZeqXu5q8ScJJFBKnxJtpBpTKYRnuduOP
Jx8mn+40skYVKF+3jBbIcAECwhiDcWpWN+ZSBpyhqxPIHOO+iAECxkB32OkhVRAW
9vfz0hpxAqTSnq0n+vsJaWSxy9VX5OjI1PHTnfzMHCSwTNScZ6kQlL2tUcoFVMLs
rP9OW1RXGg0hkztQisJF4kQeQ62keoZxoezpLoVWAh34HhhyhkOhz9ET9eLA4+V1
JeBY6ue/AgMBAAECggEANGkuJUOzsQsIKxsilYmkbPzI3kCh8W+GblaTmo/HP8WK
h6hphgEEXgqB5hm2qmJdvUyUJB3JWtNVZa48KRktLuuQXOPy0QIm1RPKRsW2FFCn
Z2Vtviyp63tHLvCPInBokFRqFbUQCBkDyk3hRc3heGCEC+ITUHy58lojv6wBsgvN
Qfy5LxWF1gtgCPLC6JNgnnBj/0tb3u+nVftc27QCBjA5PSU3HJHA9CSbraiguH1Y
M6Y0a+o8RTxxyW5+ffsuzSaxAOxIwwoonE931AArkvkxFgg50emkPOWbUnhZJeDq
9MJRJz7ADLgttI376At2RVC5baRMKa4Z9AL4oM23oQKBgQD5+eEBIjyT6HUzeUyx
7mDH88egcKLQ+1LlJyY6Sthmf4BnIScF5rvCf1HBjxZ4Zi0fEBCmSZeCD2cgrpG1
t7LzhFGh4kiF7x0k331l4N57meiCJx6NGcIR2GRF3dTUAjmuQk8fn5FOGUG/k7L9
hZrlKMIJKZwp9aYvsxaiQvHr+QKBgQDwPfPhcs9wIZGGkxd2L9r402RE6EamE/kt
HHUKU3a3yVOIrOnjlAkrN+zo215bNhQ7I+a8umjOUdUrEW02qcppGiu2W6QTCeR4
RylNfJgGLWRimp7soRiErGbyC+q/gkGrSq5ZclGFyNQwJuFbyljiCyHf3K7NWv1O
N1NiPx+vdwKBgFIKjas2llUg1N5Y8C/xgXf+bUUd0oHuCi3FJIm7KLyzGew++DS6
nmLeMHHrST+ooSRxvFUnD/+SmJEkWhQevy+m/Le5sX2rlZAVfW1jWQGN6L5WonNC
wevjbj1z6bbPKCkmABvr3d+Y8Hg0vGjyYXzWXKBvNJ6czbcX+tS0TfvZAoGBALor
KCyS7dE1EjK5FbtOhl/AYLlNTkIwxC2DGeewmhT9/K+zX2QuOZS2N+6S4GHKXI8f
2RRzV/haTdicHof3t5UO5MTh6xmd1uCmNImJfb17u4j1zSYOCJP3jacQOQ/C/uSg
cM972VTVNilCV+zrt0kj21JBD2yvkA/mq8U8qW8tAoGBAImuOUDuk6C9VlWZp/5f
LNk67JSJiTqe6bDYB6FcDUWw7j2EnhegjcxE205T1vc4BqNEJ4Ilruz7T0w+T5N9
VsxDsSwDp033fe6+XBSPEf879UZgcrq7eSqCfk+NGf2rcjcsdD8z8wd3IkqPCtKW
ICwycby2nLYd40HJv2+G3mdR
-----END PRIVATE KEY-----`
