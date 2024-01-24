package sm

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClientConfig", func() {
	When("valid ClientSecret", func() {
		It("returns true", func() {
			config := ClientConfig{
				URL:          "https://example.com",
				TokenURL:     "https://example.com/token",
				ClientID:     "validClientId",
				ClientSecret: "validClientSecret",
			}
			Expect(config.IsValid()).To(BeTrue())
		})
	})

	When("valid TLSCertKey and TLSPrivateKey", func() {
		It("returns true", func() {
			config := ClientConfig{
				URL:           "https://example.com",
				TokenURL:      "https://example.com/token",
				ClientID:      "validClientId",
				SSLDisabled:   true,
				TLSCertKey:    "CertKey",
				TLSPrivateKey: "PrivateKey",
			}
			Expect(config.IsValid()).To(BeTrue())
		})
	})

	When("no ClientSecret", func() {
		It("returns false", func() {
			config := ClientConfig{
				URL:      "https://example.com",
				TokenURL: "https://example.com/token",
				ClientID: "validClientId",
			}
			Expect(config.IsValid()).To(BeFalse())
		})
	})

	When("no TLSCertKey", func() {
		It("returns false", func() {
			config := ClientConfig{
				URL:           "https://example.com",
				TokenURL:      "https://example.com/token",
				ClientID:      "validClientId",
				SSLDisabled:   true,
				TLSPrivateKey: "PrivateKey",
			}
			Expect(config.IsValid()).To(BeFalse())
		})
	})

	When("no URL", func() {
		It("returns false", func() {
			config := ClientConfig{
				ClientID:     "validClientId",
				TokenURL:     "https://example.com/token",
				ClientSecret: "validClientSecret",
			}
			Expect(config.IsValid()).To(BeFalse())
		})
	})

	When("no ClientId", func() {
		It("returns false", func() {
			config := ClientConfig{
				URL:          "https://example.com",
				TokenURL:     "https://example.com/token",
				ClientSecret: "validClientSecret",
			}
			Expect(config.IsValid()).To(BeFalse())
		})
	})

	When("no TokenURL", func() {
		It("returns false", func() {
			config := ClientConfig{
				ClientID:     "validClientId",
				URL:          "https://example.com",
				ClientSecret: "validClientSecret",
			}
			Expect(config.IsValid()).To(BeFalse())
		})
	})
})
