package utils

import (
	"context"
	"fmt"

	"github.com/SAP/sap-btp-service-operator/client/sm"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSMClient(ctx context.Context, secretResolver *SecretResolver, resourceNamespace, btpAccessSecretName string) (sm.Client, error) {
	log := GetLogger(ctx)

	if len(btpAccessSecretName) > 0 {
		return getBTPAccessClient(ctx, secretResolver, btpAccessSecretName)
	}

	secret, err := secretResolver.GetSecretForResource(ctx, resourceNamespace, SAPBTPOperatorSecretName)
	if err != nil {
		return nil, err
	}

	clientConfig := &sm.ClientConfig{
		ClientID:       string(secret.Data["clientid"]),
		ClientSecret:   string(secret.Data["clientsecret"]),
		URL:            string(secret.Data["sm_url"]),
		TokenURL:       string(secret.Data["tokenurl"]),
		TokenURLSuffix: string(secret.Data["tokenurlsuffix"]),
		SSLDisabled:    false,
	}

	if len(clientConfig.ClientID) == 0 || len(clientConfig.URL) == 0 || len(clientConfig.TokenURL) == 0 {
		log.Info("credentials secret found but did not contain all the required data")
		return nil, fmt.Errorf("invalid Service-Manager credentials, contact your cluster administrator")
	}

	if len(clientConfig.ClientSecret) == 0 {
		tlsSecret, err := secretResolver.GetSecretForResource(ctx, resourceNamespace, SAPBTPOperatorTLSSecretName)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		if tlsSecret == nil || len(tlsSecret.Data) == 0 || len(tlsSecret.Data[v1.TLSCertKey]) == 0 || len(tlsSecret.Data[v1.TLSPrivateKeyKey]) == 0 {
			log.Info("clientsecret not found in SM credentials, and tls secret is invalid")
			return nil, fmt.Errorf("invalid Service-Manager credentials, contact your cluster administrator")
		}

		log.Info("found tls configuration")
		clientConfig.TLSCertKey = string(tlsSecret.Data[v1.TLSCertKey])
		clientConfig.TLSPrivateKey = string(tlsSecret.Data[v1.TLSPrivateKeyKey])
	}

	return sm.NewClient(ctx, clientConfig, nil)
}

func getBTPAccessClient(ctx context.Context, secretResolver *SecretResolver, secretName string) (sm.Client, error) {
	log := GetLogger(ctx)
	secret, err := secretResolver.GetSecretFromManagementNamespace(ctx, secretName)
	if err != nil {
		return nil, err
	}

	clientConfig := &sm.ClientConfig{
		ClientID:       string(secret.Data["clientid"]),
		ClientSecret:   string(secret.Data["clientsecret"]),
		URL:            string(secret.Data["sm_url"]),
		TokenURL:       string(secret.Data["tokenurl"]),
		TokenURLSuffix: string(secret.Data["tokenurlsuffix"]),
		TLSPrivateKey:  string(secret.Data[v1.TLSCertKey]),
		TLSCertKey:     string(secret.Data[v1.TLSPrivateKeyKey]),
		SSLDisabled:    false,
	}

	if !clientConfig.IsValid() {
		log.Info("btpAccess secret found but did not contain all the required data")
		return nil, fmt.Errorf("invalid Service-Manager credentials, contact your cluster administrator")
	}

	return sm.NewClient(ctx, clientConfig, nil)
}
