package httputil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"os"
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
		client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		// Load custom CA certificates if present
		if tlsConfig := loadCustomCACerts(); tlsConfig != nil {
			client.Transport.(*http.Transport).TLSClientConfig = tlsConfig
		}
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

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Load custom CA certificates if present
	if customConfig := loadCustomCACerts(); customConfig != nil {
		tlsConfig.RootCAs = customConfig.RootCAs
	}

	client.Transport.(*http.Transport).TLSClientConfig = tlsConfig

	return client, nil
}

func getClient() *http.Client {
	client := &http.Client{
		Timeout: time.Minute * 2,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	return client
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

// loadCustomCACerts loads custom CA certificates from the mounted path
// Returns nil if no custom certificates are configured
func loadCustomCACerts() *tls.Config {
	customCAPath := "/etc/ssl/certs/custom-ca-certificates.crt"

	// Check if custom CA certificate file exists
	if _, err := os.Stat(customCAPath); os.IsNotExist(err) {
		return nil
	}

	// Read the custom CA certificate file
	caCert, err := os.ReadFile(customCAPath)
	if err != nil {
		return nil
	}

	// Get system cert pool and append custom certs
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		// If system pool is unavailable, create a new pool
		rootCAs = x509.NewCertPool()
	}

	// Append custom CA certificates to the pool
	if ok := rootCAs.AppendCertsFromPEM(caCert); !ok {
		// Failed to parse certificates
		return nil
	}

	return &tls.Config{
		RootCAs: rootCAs,
	}
}
