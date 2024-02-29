package utils

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

const templateOutputMaxBytes int64 = 1 * 1024 * 1024

var validGroupVersionKind = schema.GroupVersionKind{
	Group:   "",
	Kind:    "Secret",
	Version: "v1",
}
var allowedMetadataFields = map[string]string{"labels": "any", "annotations": "any"}

// CreateSecretFromTemplate executes the template to create a secret objects, validates and returns it
// The template needs to be a v1 Secret and in metadata labels and annotations are allowed only
// Set templateOptions of the "text/template" package to specify the template behavior
func CreateSecretFromTemplate(templateName, secretTemplate string, option string, data map[string]interface{}) (*corev1.Secret, error) {

	secretManifest, err := executeTemplate(templateName, secretTemplate, option, data)
	if err != nil {
		return nil, errors.Wrap(err, "could not execute template")
	}

	yamlSerializer := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	o, err := runtime.Decode(yamlSerializer, []byte(secretManifest))
	if err != nil {
		return nil, errors.Wrapf(err, "the generated secret manifest is not a valid YAML document")
	}

	obj := o.(*unstructured.Unstructured)

	err = validateSecret(obj)
	if err != nil {
		return nil, err
	}
	var secret *corev1.Secret
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &secret)
	if err != nil {
		return nil, errors.Wrap(err, "the generated secret manifest is not valid")
	}
	return secret, nil
}

func validateSecret(obj *unstructured.Unstructured) error {
	// validate metadata

	metadataKeyValues, _, err := unstructured.NestedMap(obj.Object, "metadata")
	if err != nil {
		return errors.Wrap(err, "failed to read metadata fields of generated secret manifest")
	}

	for metadataKey := range metadataKeyValues {
		if _, ok := allowedMetadataFields[metadataKey]; !ok {
			return fmt.Errorf("metadata field %s is not allowed in generated secret manifest", metadataKey)
		}
	}

	// validate GroupVersionKind
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk != validGroupVersionKind {
		return fmt.Errorf("generated secret manifest has unexpected type: %q", gvk.String())
	}
	return nil
}

// ParseTemplate create a new template with given name, add allowed sprig functions and parse the template
func ParseTemplate(templateName, text string) (*template.Template, error) {
	return template.New(templateName).Funcs(filteredFuncMap()).Parse(text)
}

func filteredFuncMap() template.FuncMap {

	return template.FuncMap{}
}

func executeTemplate(templateName, text string, option string, parameters map[string]interface{}) (string, error) {
	t, err := ParseTemplate(templateName, text)
	if err != nil {
		return "", err
	}

	var stringBuilder strings.Builder
	var writer io.Writer = &LimitedWriter{
		W: &stringBuilder,
		N: templateOutputMaxBytes,
		Converter: func(err error) error {
			if errors.Is(err, ErrLimitExceeded) {
				return fmt.Errorf("the size of the generated secret manifest exceeds the limit of %d bytes", templateOutputMaxBytes)
			}
			return err
		},
	}
	err = t.Option(option).Execute(writer, parameters)
	if err != nil {
		return "", err
	}

	return stringBuilder.String(), nil
}
