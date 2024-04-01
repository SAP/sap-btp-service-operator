package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

const templateOutputMaxBytes int64 = 1 * 1024 * 1024

var allowedMetadataFields = map[string]string{"labels": "any", "annotations": "any", "creationTimestamp": "any"}
var validGroupVersionKind = schema.GroupVersionKind{
	Group:   "",
	Kind:    "Secret",
	Version: "v1",
}

// CreateSecretFromTemplate executes the template to create a secret objects, validates and returns it
// The template needs to be a v1 Secret and in metadata labels and annotations are allowed only
// Set templateOptions of the "text/template" package to specify the template behavior
func CreateSecretFromTemplate(templateName, secretTemplate string, option string, data map[string]interface{}) (*corev1.Secret, error) {

	secretManifest, err := executeTemplate(templateName, secretTemplate, option, data)
	if err != nil {
		return nil, errors.Wrap(err, "the Secret template is invalid")
	}

	secret := &corev1.Secret{}
	if err := yaml.Unmarshal(secretManifest, secret); err != nil {
		return nil, errors.Wrap(err, "the Secret template is invalid: It does not result in a valid Secret YAML")
	}

	if err := validateSecret(secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func validateSecret(secret *corev1.Secret) error {
	// validate GroupVersionKind
	gvk := secret.GetObjectKind().GroupVersionKind()
	if (gvk.Kind != "" || gvk.Version != "") && gvk != validGroupVersionKind {
		return fmt.Errorf("the Secret template is invalid: It is of kind '%s' but needs to be of kind 'Secret'", gvk.String())
	}

	metadataKeyValues := map[string]interface{}{}
	secretMetadataBytes, err := json.Marshal(secret.ObjectMeta)
	if err != nil {
		return errors.Wrap(err, "the Secret template is invalid: It does not result in a valid Secret YAML")
	}
	if err := json.Unmarshal(secretMetadataBytes, &metadataKeyValues); err != nil {
		return errors.Wrap(err, "the Secret template is invalid: It does not result in a valid Secret YAML")
	}

	for metadataKey := range metadataKeyValues {
		if _, ok := allowedMetadataFields[metadataKey]; !ok {
			return fmt.Errorf("the Secret template is invalid: Secret's metadata field '%s' cannot be edited", metadataKey)
		}
	}

	return nil
}

// ParseTemplate create a new template with given name, add allowed sprig functions and parse the template
func ParseTemplate(templateName, text string) (*template.Template, error) {
	return template.New(templateName).Funcs(filteredFuncMap()).Parse(text)
}

func filteredFuncMap() template.FuncMap {
	return sprig.TxtFuncMap()
}

func executeTemplate(templateName, text, option string, parameters map[string]interface{}) ([]byte, error) {
	t, err := ParseTemplate(templateName, text)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	var writer io.Writer = &LimitedWriter{
		W: &buf,
		N: templateOutputMaxBytes,
		Converter: func(err error) error {
			if errors.Is(err, ErrLimitExceeded) {
				return fmt.Errorf("the size of the generated Secret exceeds the limit of %d bytes", templateOutputMaxBytes)
			}
			return err
		},
	}
	err = t.Option(option).Execute(writer, parameters)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func GetSecretDataForTemplate(Credential map[string]interface{}, instance map[string]string) map[string]interface{} {
	return map[string]interface{}{
		common.CredentialsKey: Credential,
		common.InstanceKey:    instance,
	}
}
