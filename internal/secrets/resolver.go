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
	SAPBTPOperatorSecretName = "sap-btp-service-operator"
)

type SecretResolver struct {
	ManagementNamespace    string
	EnableNamespaceSecrets bool
	Client                 client.Client
	Log                    logr.Logger
}

func (sr *SecretResolver) GetSecretForResource(ctx context.Context, namespace string) (*v1.Secret, error) {
	var secretForResource *v1.Secret
	var err error
	found := false

	if sr.EnableNamespaceSecrets {
		sr.Log.Info("Searching for secret in resource namespace", "namespace", namespace)
		secretForResource, err = sr.getSecretFromNamespace(ctx, namespace)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		found = !apierrors.IsNotFound(err)
	}

	if !found {
		// secret not found in resource namespace, search for namespace-specific secret in management namespace
		sr.Log.Info("Searching for namespace secret in management namespace", "namespace", namespace, "managementNamespace", sr.ManagementNamespace)
		secretForResource, err = sr.getSecretForNamespace(ctx, namespace)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		found = !apierrors.IsNotFound(err)
	}

	if !found {
		// namespace-specific secret not found in management namespace, fallback to central cluster secret
		sr.Log.Info("Searching for cluster secret", "managementNamespace", sr.ManagementNamespace)
		secretForResource, err = sr.getClusterSecret(ctx)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		found = !apierrors.IsNotFound(err)
	}

	if !found {
		// secret not found anywhere
		sr.Log.Info("secret not found")
		return nil, fmt.Errorf("secret not found")
	}

	if err := validateSAPBTPOperatorSecret(secretForResource); err != nil {
		sr.Log.Error(err, "failed to validate secret", "secretName", secretForResource.Name, "secretNamespace", secretForResource.Namespace)
		return nil, err
	}

	return secretForResource, nil
}

func validateSAPBTPOperatorSecret(secret *v1.Secret) error {
	secretData := secret.Data
	if secretData == nil {
		return fmt.Errorf("invalid secret: data is missing. Check the fields and try again")
	}

	for _, field := range []string{"clientid", "clientsecret", "url", "tokenurl"} {
		if len(secretData[field]) == 0 {
			return fmt.Errorf("invalid secret: '%s' field is missing", field)
		}
	}

	return nil
}

func (sr *SecretResolver) getSecretFromNamespace(ctx context.Context, namespace string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: SAPBTPOperatorSecretName}, secret)
	return secret, err
}

func (sr *SecretResolver) getSecretForNamespace(ctx context.Context, namespace string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: sr.ManagementNamespace, Name: fmt.Sprintf("%s-%s", namespace, SAPBTPOperatorSecretName)}, secret)
	return secret, err
}

func (sr *SecretResolver) getClusterSecret(ctx context.Context) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := sr.Client.Get(ctx, types.NamespacedName{Namespace: sr.ManagementNamespace, Name: SAPBTPOperatorSecretName}, secret)
	return secret, err
}
