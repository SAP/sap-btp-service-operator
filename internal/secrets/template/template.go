package template

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	sprig "github.com/Masterminds/sprig/v3"
	"github.com/SAP/sap-btp-service-operator/internal/ioutils"
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

// CreateSecretFromTemplate executes the template to create a secret objects, validates and returns it
// The template needs to be a v1 Secret and in metadata labels and annotations are allowed only
// Set templateOptions of the "text/template" package to specify the template behavior
func CreateSecretFromTemplate(templateName, secretTemplate string, data map[string]interface{}) (*corev1.Secret, error) {

	secretManifest, err := executeTemplate(templateName, secretTemplate, data)
	if err != nil {
		return nil, errors.Wrap(err, "could not execute template")
	}

	yamlSerializer := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	o, err := runtime.Decode(yamlSerializer, []byte(secretManifest))
	if err != nil {
		return nil, errors.Wrapf(err, "the generated secret manifest is not a valid YAML document: %q", secretManifest)
	}

	obj := o.(*unstructured.Unstructured)

	// validate metadata
	allowedMetadataFields := map[string]string{"labels": "any", "annotations": "any"}

	metadataKeyValues, _, err := unstructured.NestedMap(obj.Object, "metadata")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read metadata fields of generated secret manifest")
	}

	for metadataKey := range metadataKeyValues {
		if _, ok := allowedMetadataFields[metadataKey]; !ok {
			return nil, fmt.Errorf("metadata field %s is not allowed in generated secret manifest", metadataKey)
		}
	}

	// validate GroupVersionKind
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk != validGroupVersionKind {
		return nil, fmt.Errorf("generated secret manifest has unexpected type: %q", gvk.String())
	}

	var secret *corev1.Secret
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &secret)
	if err != nil {
		return nil, errors.Wrap(err, "the generated secret manifest is not valid")
	}
	return secret, nil
}

// ParseTemplate create a new template with given name, add allowed sprig functions and parse the template
func ParseTemplate(templateName, text string) (*template.Template, error) {
	return template.New(templateName).Funcs(filteredFuncMap()).Parse(text)
}

func filteredFuncMap() template.FuncMap {
	r := sprig.TxtFuncMap()

	for sprigFunc := range r {
		if _, ok := allowedSprigFunctions[sprigFunc]; !ok {
			delete(r, sprigFunc)
		}
	}
	return r
}

func executeTemplate(templateName, text string, parameters map[string]interface{}) (string, error) {
	t, err := ParseTemplate(templateName, text)
	if err != nil {
		return "", err
	}

	var stringBuilder strings.Builder
	var writer io.Writer = &ioutils.LimitedWriter{
		W: &stringBuilder,
		N: templateOutputMaxBytes,
	}
	writer = &ioutils.ErrorConversionWriter{
		W: writer,
		Converter: func(err error) error {
			if err == ioutils.ErrLimitExceeded {
				return fmt.Errorf("the size of the generated secret manifest exceeds the limit of %d bytes", templateOutputMaxBytes)
			}
			return err
		},
	}

	err = t.Option("missingkey=error").Execute(writer, parameters)
	if err != nil {
		return "", err
	}

	return stringBuilder.String(), nil
}
