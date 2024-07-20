package utils

import (
	"context"
	"fmt"

	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type InvalidCredentialsError struct{}

func (ic *InvalidCredentialsError) Error() string {
	return "invalid Service-Manager credentials, contact your cluster administrator"
}

func GetSMClient(ctx context.Context, k8sClient client.Client, serviceInstance *v1.ServiceInstance) (sm.Client, error) {
	log := GetLogger(ctx)
	var err error

	secret := &corev1.Secret{}
	if serviceInstance.Status.SecretRef != nil {
		if err = GetSecretWithFallback(ctx, serviceInstance.Status.SecretRef.ToNamespaceName(), secret); err != nil {
			log.Error(err, "failed to get secret using status.secretRef")
			return nil, err
		}
	} else {
		if len(serviceInstance.Spec.BTPAccessCredentialsSecret) > 0 {
			secret, err = GetSecretFromManagementNamespace(ctx, serviceInstance.Spec.BTPAccessCredentialsSecret)
			if err != nil {
				log.Error(err, "failed to get secret BTPAccessCredentialsSecret")
				return nil, err
			}
		} else {
			secret, err = GetSecretForResource(ctx, serviceInstance.Namespace, SAPBTPOperatorSecretName)
			if err != nil {
				log.Error(err, "failed to get secret for instance")
				return nil, err
			}
		}

		serviceInstance.Status.SecretRef = &v1.ResourceKey{Name: secret.Name, Namespace: secret.Namespace}
		if err = k8sClient.Status().Update(ctx, serviceInstance); err != nil {
			log.Error(err, "failed to update status with secretRef")
			return nil, err
		}
	}

	clientConfig := &sm.ClientConfig{
		ClientID:       string(secret.Data["clientid"]),
		ClientSecret:   string(secret.Data["clientsecret"]),
		URL:            string(secret.Data["sm_url"]),
		TokenURL:       string(secret.Data["tokenurl"]),
		TokenURLSuffix: string(secret.Data["tokenurlsuffix"]),
		TLSPrivateKey:  string(secret.Data[corev1.TLSPrivateKeyKey]),
		TLSCertKey:     string(secret.Data[corev1.TLSCertKey]),
		SSLDisabled:    false,
	}

	if len(clientConfig.ClientID) == 0 || len(clientConfig.URL) == 0 || len(clientConfig.TokenURL) == 0 {
		log.Info("credentials secret found but did not contain all the required data")
		return nil, fmt.Errorf("invalid Service-Manager credentials, contact your cluster administrator")
	}

	//backward compatibility (tls data in a dedicated secret)
	if len(clientConfig.ClientSecret) == 0 && (len(clientConfig.TLSPrivateKey) == 0 || len(clientConfig.TLSCertKey) == 0) {
		if len(serviceInstance.Spec.BTPAccessCredentialsSecret) > 0 && !clientConfig.IsValid() {
			log.Info("btpAccess secret found but did not contain all the required data")
			return nil, fmt.Errorf("invalid Service-Manager credentials, contact your cluster administrator")
		}

		tlsSecret, err := GetSecretForResource(ctx, serviceInstance.Namespace, SAPBTPOperatorTLSSecretName)
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}

		if tlsSecret == nil || len(tlsSecret.Data) == 0 || len(tlsSecret.Data[corev1.TLSCertKey]) == 0 || len(tlsSecret.Data[corev1.TLSPrivateKeyKey]) == 0 {
			log.Info("clientsecret not found in SM credentials, and tls secret is invalid")
			return nil, &InvalidCredentialsError{}
		}

		log.Info("found tls configuration")
		clientConfig.TLSCertKey = string(tlsSecret.Data[corev1.TLSCertKey])
		clientConfig.TLSPrivateKey = string(tlsSecret.Data[corev1.TLSPrivateKeyKey])
	}

	return sm.NewClient(ctx, clientConfig, nil)
}
