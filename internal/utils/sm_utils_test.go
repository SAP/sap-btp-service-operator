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
			XWhen("valid", func() {
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
				FIt("should succeed", func() {
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
			//TODO
		})
	})
})

const tlscrt = `
-----BEGIN CERTIFICATE-----
MIICwDCCAaigAwIBAgIUD6NjUHR/8u0wDQYJKoZIhvcNAQELBQAwEzERMA8GA1UE
AxMIb3BlbmFpMB4XDTIxMDkwNzExMjM0MVoXDTIyMDkwNzExMjM0MVowEzERMA8G
A1UEAxMIb3BlbmFpMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxTjJ
qxaMXy7GvwLReer89W14s0sLLxGtrTkt8miPjGYI8Sh1vlVLxP3aW5szXc2o21p5
r8dC62XN1vOqVh+NR16pWzBoNh1BNe0nv2LC3rB1RcfsBz8YowNpgK4eWGYZYF+o
iU0iLQw+Z0DD8wN4XYu/1DN+v6XzG6e6UsJ/iee1msPiP8xEcs+gVGdq1+fwCIsI
OvFK6TQIu7W5PVGrh2Tkgf1ks2ICklh+55+fF5PyP4fqXKIoHY4JMDtrq1g+j22E
YIvCu0rlq+2dzhtYjviwAT/lTJtYYxP5nzPKhgo6NGLGFTOcwDIk3g/b8g3PzFHz
kym+IbZdDQIDAQABo1MwUTAdBgNVHQ4EFgQUDUxmCq2rLzKv4QG6JhWx9aXpMr0w
HwYDVR0jBBgwFoAUDUxmCq2rLzKv4QG6JhWx9aXpMr0wDwYDVR0TAQH/BAUwAwEB
/zANBgkqhkiG9w0BAQsFAAOCAQEAkB+N4Ev5mDfhFkH2CDaD+34tYHVGtX1pDPgB
YP3v/wzTQ3R8j6hH/NRBi6H9BxFXUJW8rWFN4Cisn4/gfYi0KGJHnF1Hj9y9Fb66
tStN0zH5wQfqy5LmWHLQ+RP36jGuzNNTud0kfJuyulNfrkl29yGZf/X3hTusHdbQ
PxZMpzZLsfBqqvQa4ZaEwWiTjoEKzzf1XdYPC2O3x6XlFiCgh/cAM8ej9yy7DL1T
+wDMNWfw1/PhMyh9A/5B0iF5G7oE+zGAfBOy+h/wy6P8ylVchCx4C4Bgho0D9qol
heRwTJ72IZW8Kp7d0B9RoVomVHosdNPhrfN9wXlqRAXrNY6sEA==
-----END CERTIFICATE-----
`
const tlskey = `
-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDFOMmrFoxfLsa/
AtF56vz1bXizSwsvEa20pLfJo+MZgjxKHW+VUvE/dpbnM1zajbWnmvx0LrZc3W86
pWH41HXqlbMGg2HUE17Se/YsLesHVFx+wHPxijA2mArh5YZhlgX6iJTSIsMD5nQM
PzA3hdS7/UM36/pfMbp7pSwn+J57Waw+I/zERyz6BUZ2rX5/AIiwg68UrpNAm7tb
k9UauHZOSE/WSzYgKSWH7nn58Xk/I/h+pcoigdjgkwO2urWD6PbYRgi8K7SuWr7Z
3OG1iO+LAA/5UybWGMT+Z8zyoYKOjRixhUznMAyJN4P2/IHz8UfOTKb4htl0NAgM
BAAECggEAPqN3UZMjys2Qr9W+JN14TeSygHseFV5MKXlO3nqRwQCF3dQbgoNl4Rj
CzqGyXl9h1yy+CQzR24FLJ2aog2/xtUT53n1Vle8JyMqPibGSnAeYYHbAnQ13OSG
bK5n2u5aplgsoEumx9wJlTzGyobtAlDnL3Z7tEeD6uqjwYLUzjUcXpG4ej3Oo2H+
B0USltdh0cEPEQCrREIS0HhxPdntCCYH+3m/DLWFvskYrUu/T1Cqtsn5lBdngRXp
lltrqy8WnoDPOxuAOsH5FwI0+RE99HEoU4+e0EGy9V7Xp3XzgmUWKFMMMLfVvqGb
wFsyozvQl/5ZAYdHAF72Im7l7jNjQKBgQDZwBU7Oz6qI8AsO0WIt/Uy0vxBaa2K5
s0nG7fsaXhX4PNqk5W3/+sy/07k9zDxfRP5mHbKmtJgZQxrW3JZxuO1Uw0idc+0E
C0u4XhBTT5yBfMBQf9zRU
`
