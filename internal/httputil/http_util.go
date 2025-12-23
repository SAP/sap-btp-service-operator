package httputil

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

// NormalizeURL removes trailing slashesh in url
func NormalizeURL(url string) string {
	for strings.HasSuffix(url, "/") {
		url = url[:len(url)-1]
	}
	return url
}

// BuildHTTPClient builds custom http client with configured ssl validation
func BuildHTTPClient(sslDisabled bool) *http.Client {
	client := getClient()
	if sslDisabled {
		// Preserve FIPS compliance even when skipping verification
		tlsConfig := getFipsCompliantTLSConfig()
		tlsConfig.InsecureSkipVerify = true
		client.Transport.(*http.Transport).TLSClientConfig = tlsConfig
	}

	return client
}

// BuildHTTPClientTLS BuildHTTPClient builds custom http client with configured ssl validation
func BuildHTTPClientTLS(tlsCertKey, tlsPrivateKey string) (*http.Client, error) {
	client := getClient()

	cert, err := tls.X509KeyPair([]byte(tlsCertKey), []byte(tlsPrivateKey))
	if err != nil {
		return nil, err
	}

	// Start with FIPS-compliant config and add the client certificate
	tlsConfig := getFipsCompliantTLSConfig()
	tlsConfig.Certificates = []tls.Certificate{cert}
	client.Transport.(*http.Transport).TLSClientConfig = tlsConfig

	return client, nil
}

func getClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       getFipsCompliantTLSConfig(),
	}

	client := &http.Client{
		Timeout:   time.Minute * 2,
		Transport: transport,
	}
	return client
}

// getFipsCompliantTLSConfig returns a FIPS-compliant TLS configuration
func getFipsCompliantTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.CurveP384,
			tls.CurveP521,
		},
	}
}

// UnmarshalResponse reads the response body and tries to parse it.
func UnmarshalResponse(response *http.Response, jsonResult interface{}) error {

	defer func() {
		err := response.Body.Close()
		if err != nil {
			panic(err)
		}
	}()

	return json.NewDecoder(response.Body).Decode(&jsonResult)
}
