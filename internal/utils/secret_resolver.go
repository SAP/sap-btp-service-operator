package utils

import (
	"context"
	"fmt"

	"github.com/SAP/sap-btp-service-operator/internal/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//TODO + revisit the name based approach for managed secret, replace with label based mechanism + admission webhook for secrets to avoid duplications

const (
	SAPBTPOperatorSecretName    = "sap-btp-service-operator"
	SAPBTPOperatorTLSSecretName = "sap-btp-service-operator-tls"
)

var secretsClient secretClient

type secretClient struct {
	ManagementNamespace    string
	ReleaseNamespace       string
	EnableNamespaceSecrets bool
	LimitedCacheEnabled    bool
	Client                 client.Client
	NonCachedClient        client.Client
	Log                    logr.Logger
}

func InitializeSecretsClient(client, nonCachedClient client.Client, config config.Config) {
	secretsClient = secretClient{
		Log:                    logf.Log.WithName("secret-resolver"),
		ManagementNamespace:    config.ManagementNamespace,
		ReleaseNamespace:       config.ReleaseNamespace,
		EnableNamespaceSecrets: config.EnableNamespaceSecrets,
		LimitedCacheEnabled:    config.EnableLimitedCache,
		Client:                 client,
		NonCachedClient:        nonCachedClient,
	}
}

func GetSecretWithFallback(ctx context.Context, namespacedName types.NamespacedName, secret *v1.Secret) error {
	return secretsClient.getWithClientFallback(ctx, namespacedName, secret)
}

func GetSecretFromManagementNamespace(ctx context.Context, name string) (*v1.Secret, error) {
	return secretsClient.getSecretFromManagementNamespace(ctx, name)
}

func GetSecretForResource(ctx context.Context, namespace, name string) (*v1.Secret, error) {
	return secretsClient.getSecretForResource(ctx, namespace, name)
}

func (sr *secretClient) getSecretFromManagementNamespace(ctx context.Context, name string) (*v1.Secret, error) {
	secretForResource := &v1.Secret{}

	err := sr.getWithClientFallback(ctx, types.NamespacedName{Name: name, Namespace: sr.ManagementNamespace}, secretForResource)
	if err != nil {
		return nil, err
	}
	return secretForResource, nil
}

func (sr *secretClient) getSecretForResource(ctx context.Context, namespace, name string) (*v1.Secret, error) {
	secretForResource := &v1.Secret{}

	// search namespace secret
	if sr.EnableNamespaceSecrets {
		err := sr.getWithClientFallback(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secretForResource)
		if err == nil {
			return secretForResource, nil
		}

		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
	}

	// secret not found in resource namespace, search for namespace-specific secret in management namespace
	var err error
	secretForResource, err = secretsClient.getSecretFromManagementNamespace(ctx, fmt.Sprintf("%s-%s", namespace, name))
	if err == nil {
		return secretForResource, nil
	}

	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	// namespace-specific secret not found in management namespace, fallback to central cluster secret
	return sr.getClusterDefaultSecret(ctx, name)
}

func (sr *secretClient) getClusterDefaultSecret(ctx context.Context, name string) (*v1.Secret, error) {
	secretForResource := &v1.Secret{}
	//	sr.Log.Info(fmt.Sprintf("Searching for cluster secret %s in releaseNamespace %s", name, sr.ReleaseNamespace))
	err := sr.getWithClientFallback(ctx, types.NamespacedName{Namespace: sr.ReleaseNamespace, Name: name}, secretForResource)
	if err != nil {
		sr.Log.Error(err, fmt.Sprintf("Could not fetch cluster secret %s from releaseNamespace %s", name, sr.ReleaseNamespace))
		return nil, err
	}
	return secretForResource, nil
}

func (sr *secretClient) getWithClientFallback(ctx context.Context, key types.NamespacedName, secretForResource *v1.Secret) error {
	err := sr.Client.Get(ctx, key, secretForResource)
	if err != nil {
		if errors.IsNotFound(err) && sr.LimitedCacheEnabled {
			err = sr.NonCachedClient.Get(ctx, key, secretForResource)
			if err != nil {
				return err
			}
			return nil
		}
		return err
	}

	return nil
}
