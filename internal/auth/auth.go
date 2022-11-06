package auth

import (
	"context"
	"net/http"

	"github.com/SAP/sap-btp-service-operator/internal/httputil"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// HTTPClient interface
//
//go:generate counterfeiter . HTTPClient
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewAuthClient(ccConfig *clientcredentials.Config, sslDisabled bool) HTTPClient {
	httpClient := httputil.BuildHTTPClient(sslDisabled)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	return oauth2.NewClient(ctx, ccConfig.TokenSource(ctx))
}

func NewAuthClientWithTLS(ccConfig *clientcredentials.Config, tlsCertKey, tlsPrivateKey string) (HTTPClient, error) {
	httpClient, err := httputil.BuildHTTPClientTLS(tlsCertKey, tlsPrivateKey)
	if err != nil {
		return nil, err
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	return oauth2.NewClient(ctx, ccConfig.TokenSource(ctx)), nil
}
