package controllers

import (
	"context"
	"encoding/json"

	"fmt"

	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// buildParameters generates the parameters JSON structure to be passed
// to the broker.
// The first return value is a map of parameters to send to the Broker, including
// secret values.
// The second return value is parameters marshalled to byt array
// The third return value is any error that caused the function to fail.
func buildParameters(kubeClient client.Client, namespace string, parametersFrom []servicesv1.ParametersFromSource, parameters *runtime.RawExtension) (map[string]interface{}, []byte, error) {
	params := make(map[string]interface{})
	if len(parametersFrom) > 0 {
		for _, p := range parametersFrom {
			fps, err := fetchParametersFromSource(kubeClient, namespace, &p)
			if err != nil {
				return nil, nil, err
			}
			for k, v := range fps {
				// we don't want to add shared param because sm api does not support updating
				// shared param with other params, for sharing we have different function.
				if k == "shared" {
					continue
				}
				if _, ok := params[k]; ok {
					return nil, nil, fmt.Errorf("conflict: duplicate entry for parameter %q", k)
				}
				params[k] = v
			}
		}
	}
	if parameters != nil {
		pp, err := UnmarshalRawParameters(parameters.Raw)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range pp {
			if _, ok := params[k]; ok {
				return nil, nil, fmt.Errorf("conflict: duplicate entry for parameter %q", k)
			}
			params[k] = v
		}
	}
	// Replace empty map with nil so that the params are omitted from the request
	if len(params) == 0 {
		params = nil
	}

	parametersRaw, err := MarshalRawParameters(params)
	if err != nil {
		return nil, nil, err
	}
	return params, parametersRaw, nil
}

// fetchParametersFromSource fetches data from a specified external source and
// represents it in the parameters map format
func fetchParametersFromSource(kubeClient client.Client, namespace string, parametersFrom *servicesv1.ParametersFromSource) (map[string]interface{}, error) {
	var params map[string]interface{}
	if parametersFrom.SecretKeyRef != nil {
		data, err := fetchSecretKeyValue(kubeClient, namespace, parametersFrom.SecretKeyRef)
		if err != nil {
			return nil, err
		}
		p, err := unmarshalJSON(data)
		if err != nil {
			return nil, err
		}
		params = p

	}
	return params, nil
}

// UnmarshalRawParameters produces a map structure from a given raw YAML/JSON input
func UnmarshalRawParameters(in []byte) (map[string]interface{}, error) {
	parameters := make(map[string]interface{})
	if len(in) > 0 {
		if err := yaml.Unmarshal(in, &parameters); err != nil {
			return parameters, err
		}
	}
	return parameters, nil
}

// MarshalRawParameters marshals the specified map of parameters into JSON
func MarshalRawParameters(in map[string]interface{}) ([]byte, error) {
	if len(in) == 0 {
		return nil, nil
	}
	return json.Marshal(in)
}

// unmarshalJSON produces a map structure from a given raw JSON input
func unmarshalJSON(in []byte) (map[string]interface{}, error) {
	parameters := make(map[string]interface{})
	if err := json.Unmarshal(in, &parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters as JSON object: %v", err)
	}
	return parameters, nil
}

// fetchSecretKeyValue requests and returns the contents of the given secret key
func fetchSecretKeyValue(kubeClient client.Client, namespace string, secretKeyRef *servicesv1.SecretKeyReference) ([]byte, error) {
	secret := &corev1.Secret{}
	err := kubeClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: secretKeyRef.Name}, secret)

	if err != nil {
		return nil, err
	}
	return secret.Data[secretKeyRef.Key], nil
}
