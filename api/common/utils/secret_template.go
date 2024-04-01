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

// genericMap from https://github.com/Masterminds/sprig/blob/master/functions.go
var allowedSprigFunctions = map[string]interface{}{
	"hello": nil,

	// Date functions
	"ago":              nil,
	"date":             nil,
	"date_in_zone":     nil,
	"date_modify":      nil,
	"dateInZone":       nil,
	"dateModify":       nil,
	"duration":         nil,
	"durationRound":    nil,
	"htmlDate":         nil,
	"htmlDateInZone":   nil,
	"must_date_modify": nil,
	"mustDateModify":   nil,
	"mustToDate":       nil,
	"now":              nil,
	"toDate":           nil,
	"unixEpoch":        nil,

	// Strings
	"abbrev":     nil,
	"abbrevboth": nil,
	"trunc":      nil,
	"trim":       nil,
	"upper":      nil,
	"lower":      nil,
	"title":      nil,
	"untitle":    nil,
	"substr":     nil,
	// Switch order so that "foo" | repeat 5
	"repeat": nil,
	// Deprecated: Use trimAll.
	//"trimall": nil,
	// Switch order so that "$foo" | trimall "$"
	"trimAll":      nil,
	"trimSuffix":   nil,
	"trimPrefix":   nil,
	"nospace":      nil,
	"initials":     nil,
	"randAlphaNum": nil,
	"randAlpha":    nil,
	"randAscii":    nil,
	"randNumeric":  nil,
	"swapcase":     nil,
	"shuffle":      nil,
	"snakecase":    nil,
	"camelcase":    nil,
	"kebabcase":    nil,
	"wrap":         nil,
	"wrapWith":     nil,
	// Switch order so that "foobar" | contains "foo"
	"contains":   nil,
	"hasPrefix":  nil,
	"hasSuffix":  nil,
	"quote":      nil,
	"squote":     nil,
	"cat":        nil,
	"indent":     nil,
	"nindent":    nil,
	"replace":    nil,
	"plural":     nil,
	"sha1sum":    nil,
	"sha256sum":  nil,
	"adler32sum": nil,
	"toString":   nil,

	// Wrap Atoi to stop errors.
	"atoi":      nil,
	"int64":     nil,
	"int":       nil,
	"float64":   nil,
	"seq":       nil,
	"toDecimal": nil,

	// split "/" foo/bar returns map[int]string{0: foo, 1: bar}
	"split":     nil,
	"splitList": nil,
	// splitn "/" foo/bar/fuu returns map[int]string{0: foo, 1: bar/fuu}
	"splitn":    nil,
	"toStrings": nil,

	"until":     nil,
	"untilStep": nil,

	// VERY basic arithmetic.
	"add1":    nil,
	"add":     nil,
	"sub":     nil,
	"div":     nil,
	"mod":     nil,
	"mul":     nil,
	"randInt": nil,
	"add1f":   nil,
	"addf":    nil,
	"subf":    nil,
	"divf":    nil,
	"mulf":    nil,
	"biggest": nil,
	"max":     nil,
	"min":     nil,
	"maxf":    nil,
	"minf":    nil,
	"ceil":    nil,
	"floor":   nil,
	"round":   nil,

	// string slices. Note that we reverse the order b/c that's better
	// for template processing.
	"join":      nil,
	"sortAlpha": nil,

	// Defaults
	"default":          nil,
	"empty":            nil,
	"coalesce":         nil,
	"all":              nil,
	"any":              nil,
	"compact":          nil,
	"mustCompact":      nil,
	"fromJson":         nil,
	"toJson":           nil,
	"toPrettyJson":     nil,
	"toRawJson":        nil,
	"mustFromJson":     nil,
	"mustToJson":       nil,
	"mustToPrettyJson": nil,
	"mustToRawJson":    nil,
	"ternary":          nil,
	"deepCopy":         nil,
	"mustDeepCopy":     nil,

	// Reflection
	"typeOf":     nil,
	"typeIs":     nil,
	"typeIsLike": nil,
	"kindOf":     nil,
	"kindIs":     nil,
	"deepEqual":  nil,

	// OS:
	// "env":       nil,
	// "expandenv": nil,

	// Network:
	// "getHostByName": nil,

	// Paths:
	"base":  nil,
	"dir":   nil,
	"clean": nil,
	"ext":   nil,
	"isAbs": nil,

	// Filepaths:
	// "osBase":  nil,
	// "osClean": nil,
	// "osDir":   nil,
	// "osExt":   nil,
	// "osIsAbs": nil,

	// Encoding:
	"b64enc": nil,
	"b64dec": nil,
	"b32enc": nil,
	"b32dec": nil,

	// Data Structures:
	"tuple":              nil, // FIXME: with the addition of append/prepend these are no longer immutable.
	"list":               nil,
	"dict":               nil,
	"get":                nil,
	"set":                nil,
	"unset":              nil,
	"hasKey":             nil,
	"pluck":              nil,
	"keys":               nil,
	"pick":               nil,
	"omit":               nil,
	"merge":              nil,
	"mergeOverwrite":     nil,
	"mustMerge":          nil,
	"mustMergeOverwrite": nil,
	"values":             nil,

	"append": nil, "push": nil,
	"mustAppend": nil, "mustPush": nil,
	"prepend":     nil,
	"mustPrepend": nil,
	"first":       nil,
	"mustFirst":   nil,
	"rest":        nil,
	"mustRest":    nil,
	"last":        nil,
	"mustLast":    nil,
	"initial":     nil,
	"mustInitial": nil,
	"reverse":     nil,
	"mustReverse": nil,
	"uniq":        nil,
	"mustUniq":    nil,
	"without":     nil,
	"mustWithout": nil,
	"has":         nil,
	"mustHas":     nil,
	"slice":       nil,
	"mustSlice":   nil,
	"concat":      nil,
	"dig":         nil,
	"chunk":       nil,
	"mustChunk":   nil,

	// Crypto:
	// "bcrypt":                   nil,
	// "htpasswd":                 nil,
	// "genPrivateKey":            nil,
	// "derivePassword":           nil,
	// "buildCustomCert":          nil,
	// "genCA":                    nil,
	// "genCAWithKey":             nil,
	// "genSelfSignedCert":        nil,
	// "genSelfSignedCertWithKey": nil,
	// "genSignedCert":            nil,
	// "genSignedCertWithKey":     nil,
	// "encryptAES":               nil,
	// "decryptAES":               nil,
	// "randBytes":                nil,

	// UUIDs:
	"uuidv4": nil,

	// SemVer:
	"semver":        nil,
	"semverCompare": nil,

	// Flow Control:
	"fail": nil,

	// Regex
	"regexMatch":                 nil,
	"mustRegexMatch":             nil,
	"regexFindAll":               nil,
	"mustRegexFindAll":           nil,
	"regexFind":                  nil,
	"mustRegexFind":              nil,
	"regexReplaceAll":            nil,
	"mustRegexReplaceAll":        nil,
	"regexReplaceAllLiteral":     nil,
	"mustRegexReplaceAllLiteral": nil,
	"regexSplit":                 nil,
	"mustRegexSplit":             nil,
	"regexQuoteMeta":             nil,

	// URLs:
	"urlParse": nil,
	"urlJoin":  nil,
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
	funcs := sprig.TxtFuncMap()

	for sprigFunc := range funcs {
		if _, ok := allowedSprigFunctions[sprigFunc]; !ok {
			delete(funcs, sprigFunc)
		}
	}
	return funcs
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
