package secrets

import (
	"context"

	"fmt"

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

type SecretResolver struct {
	ManagementNamespace    string
	ReleaseNamespace       string
	EnableNamespaceSecrets bool
	Client                 client.Client
	Log                    logr.Logger
}

func (sr *SecretResolver) GetSecretForResource(ctx context.Context, namespace, name, btpAccessSecret string) (*v1.Secret, error) {
	secretForResource := &v1.Secret{}

	if len(btpAccessSecret) > 0 {
		sr.Log.Info(fmt.Sprintf("Searching for secret name %s in namespace %s",
			btpAccessSecret, sr.ManagementNamespace))
		err := sr.Client.Get(ctx, types.NamespacedName{Name: btpAccessSecret, Namespace: sr.ManagementNamespace}, secretForResource)
		if err != nil {
			sr.Log.Error(err, fmt.Sprintf("Could not fetch secret named %s", btpAccessSecret))
			return nil, err
		}
		return secretForResource, nil
	}

	// search namespace secret
	if sr.EnableNamespaceSecrets {
		sr.Log.Info("Searching for secret in resource namespace", "namespace", namespace, "name", name)
		err := sr.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secretForResource)
		if err == nil {
			return secretForResource, nil
		}

		if client.IgnoreNotFound(err) != nil {
			sr.Log.Error(err, "Could not fetch secret in resource namespace")
			return nil, err
		}
	}

	// secret not found in resource namespace, search for namespace-specific secret in management namespace
	sr.Log.Info("Searching for namespace secret in management namespace", "namespace", namespace, "managementNamespace", sr.ManagementNamespace, "name", name)
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: sr.ManagementNamespace, Name: fmt.Sprintf("%s-%s", namespace, name)}, secretForResource)
	if err == nil {
		return secretForResource, nil
	}

	if client.IgnoreNotFound(err) != nil {
		sr.Log.Error(err, "Could not fetch secret in management namespace")
		return nil, err
	}

	// namespace-specific secret not found in management namespace, fallback to central cluster secret
	return sr.getDefaultSecret(ctx, name)
}

func (sr *SecretResolver) getDefaultSecret(ctx context.Context, name string) (*v1.Secret, error) {
	secretForResource := &v1.Secret{}
	sr.Log.Info("Searching for cluster secret", "releaseNamespace", sr.ReleaseNamespace, "name", name)
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: sr.ReleaseNamespace, Name: name}, secretForResource)
	if err != nil {
		sr.Log.Error(err, "Could not fetch cluster secret")
		return nil, err
	}
	return secretForResource, nil
}
