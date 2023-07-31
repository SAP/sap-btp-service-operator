package secrets

import (
	"context"

	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

func (sr *SecretResolver) GetSecretForResource(ctx context.Context, namespace, name string) (*v1.Secret, error) {
	var secretForResource *v1.Secret
	var err error
	found := false

	if sr.EnableNamespaceSecrets {
		sr.Log.Info("Searching for secret in resource namespace", "namespace", namespace, "name", name)
		secretForResource, err = sr.getSecretFromNamespace(ctx, namespace, name)
		if client.IgnoreNotFound(err) != nil {
			sr.Log.Error(err, "Could not fetch secret in resource namespace")
			return nil, err
		}

		found = !apierrors.IsNotFound(err)
	}

	if !found {
		// secret not found in resource namespace, search for namespace-specific secret in management namespace
		sr.Log.Info("Searching for namespace secret in management namespace", "namespace", namespace, "managementNamespace", sr.ManagementNamespace, "name", name)
		secretForResource, err = sr.getSecretForNamespace(ctx, namespace, name)
		if client.IgnoreNotFound(err) != nil {
			sr.Log.Error(err, "Could not fetch secret in management namespace")
			return nil, err
		}

		found = !apierrors.IsNotFound(err)
	}

	if !found {
		// namespace-specific secret not found in management namespace, fallback to central cluster secret
		sr.Log.Info("Searching for cluster secret", "releaseNamespace", sr.ReleaseNamespace, "name", name)
		secretForResource, err = sr.getClusterSecret(ctx, name)
		if err != nil {
			sr.Log.Error(err, "Could not fetch cluster secret")
			return nil, err
		}
	}

	return secretForResource, nil
}

func (sr *SecretResolver) getSecretFromNamespace(ctx context.Context, namespace, name string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, secret)
	return secret, err
}

func (sr *SecretResolver) getSecretForNamespace(ctx context.Context, namespace, name string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: sr.ManagementNamespace, Name: fmt.Sprintf("%s-%s", namespace, name)}, secret)
	return secret, err
}

func (sr *SecretResolver) getClusterSecret(ctx context.Context, name string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: sr.ReleaseNamespace, Name: name}, secret)
	return secret, err
}
