package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"

	"github.com/SAP/sap-btp-service-operator/internal/httputil"
	"github.com/SAP/sap-btp-service-operator/internal/utils/logutils"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const CustomCAPath = "/etc/ssl/certs/custom-ca-certificates.crt"

// HTTPClient interface
//
//go:generate counterfeiter . HTTPClient
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewAuthClient(ctx context.Context, ccConfig *clientcredentials.Config, sslDisabled bool) HTTPClient {
	httpClient := httputil.BuildHTTPClient(sslDisabled)
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	client, _ := newHTTPClient(ctxWithClient, ccConfig)
	return client
}

func NewAuthClientWithTLS(ctx context.Context, ccConfig *clientcredentials.Config, tlsCertKey, tlsPrivateKey string) (HTTPClient, error) {
	httpClient, err := httputil.BuildHTTPClientTLS(tlsCertKey, tlsPrivateKey)
	if err != nil {
		return nil, err
	}
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	return newHTTPClient(ctxWithClient, ccConfig)
}

func newHTTPClient(ctx context.Context, ccConfig *clientcredentials.Config) (HTTPClient, error) {
	log := logutils.GetLogger(ctx)
	log.Info("creating new HTTP client with OAuth2")
	client := oauth2.NewClient(ctx, ccConfig.TokenSource(ctx))
	if caPEM, err := os.ReadFile(CustomCAPath); err == nil {
		log.Info("found custom CA, loading it..")
		certPool, certPoolErr := x509.SystemCertPool()
		if certPoolErr != nil {
			// If system pool is unavailable, create a new pool
			log.Error(certPoolErr, "system cert pool is unavailable, using a new pool")
			certPool = x509.NewCertPool()
		}
		if ok := certPool.AppendCertsFromPEM(caPEM); !ok {
			log.Error(nil, "no certificates parsed from custom CA bundle")
			return nil, errors.New("invalid custom CA certificates")
		}

		oauthTransport, ok := client.Transport.(*oauth2.Transport)
		if !ok {
			log.Error(errors.New("Internal Server Error"), "unable to cast http.Client Transport to oauth2.Transport")
			return nil, errors.New("Internal Server Error")
		}

		baseTransport, ok := oauthTransport.Base.(*http.Transport)
		if !ok {
			log.Info("http.Client Transport base is not http.Transport, using default transport")
			baseTransport = http.DefaultTransport.(*http.Transport).Clone()
		} else {
			baseTransport = baseTransport.Clone()
		}
		if baseTransport.TLSClientConfig == nil {
			baseTransport.TLSClientConfig = &tls.Config{}
		}
		baseTransport.TLSClientConfig.RootCAs = certPool
		oauthTransport.Base = baseTransport
	} else if !os.IsNotExist(err) {
		log.Error(err, "failed to read customCA pem")
		return nil, errors.New("invalid custom CA")
	}

	return client, nil
}
