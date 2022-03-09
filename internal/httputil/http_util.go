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
		client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return client
}

// BuildHTTPClient builds custom http client with configured ssl validation
func BuildHTTPClientTLS(tlsCertKey, tlsPrivateKey string) (*http.Client, error) {
	client := getClient()

	cert, err := tls.X509KeyPair([]byte(tlsCertKey), []byte(tlsPrivateKey))
	if err != nil {
		return nil, err
	}

	client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	return client, nil
}

func getClient() *http.Client {
	client := &http.Client{
		Timeout: time.Second * 10,
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
