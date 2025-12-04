package auth

import (
	"context"
	"crypto/x509"
	"net/http"
	"os"

	"github.com/SAP/sap-btp-service-operator/internal/httputil"
	"github.com/SAP/sap-btp-service-operator/internal/utils/log_utils"
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

func NewAuthClient(ccConfig *clientcredentials.Config, sslDisabled bool) HTTPClient {
	httpClient := httputil.BuildHTTPClient(sslDisabled)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	client, _ := newHttpClient(ctx, ccConfig)
	return client
}

func NewAuthClientWithTLS(ccConfig *clientcredentials.Config, tlsCertKey, tlsPrivateKey string) (HTTPClient, error) {
	httpClient, err := httputil.BuildHTTPClientTLS(tlsCertKey, tlsPrivateKey)
	if err != nil {
		return nil, err
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, httpClient)
	return newHttpClient(ctx, ccConfig)
}

func newHttpClient(ctx context.Context, ccConfig *clientcredentials.Config) (HTTPClient, error) {
	log := log_utils.GetLogger(ctx)
	client := oauth2.NewClient(ctx, ccConfig.TokenSource(ctx))
	if caPEM, err := os.ReadFile(CustomCAPath); err == nil {
		log.Info("found custom CA, loading it..")
		certPool, certPoolErr := x509.SystemCertPool()
		if certPoolErr != nil {
			// If system pool is unavailable, create a new pool
			log.Error(certPoolErr, "system cert pool is unavailable, using a new pool")
			certPool = x509.NewCertPool()
		}
		certPool.AppendCertsFromPEM(caPEM)

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

		baseTransport.TLSClientConfig.RootCAs = certPool
		oauthTransport.Base = baseTransport
	} else if !os.IsNotExist(err) {
		log.Error(err, "failed to read customCA pem")
		return nil, errors.New("invalid custom CA")
	}

	return client, nil
}
