package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/util/rand"

	v1 "k8s.io/api/authentication/v1"
)

const (
	TEXT    format = "text"
	JSON    format = "json"
	UNKNOWN format = "unknown"
)

type SecretMetadataProperty struct {
	Name      string `json:"name"`
	Container bool   `json:"container,omitempty"`
	Format    string `json:"format"`
}

type format string

type LogKey struct {
}

func RemoveFinalizer(ctx context.Context, k8sClient client.Client, object common.SAPBTPResource, finalizerName string) error {
	log := GetLogger(ctx)
	if controllerutil.ContainsFinalizer(object, finalizerName) {
		log.Info(fmt.Sprintf("removing finalizer %s", finalizerName))
		controllerutil.RemoveFinalizer(object, finalizerName)
		if err := k8sClient.Update(ctx, object); err != nil {
			if err := k8sClient.Get(ctx, apimachinerytypes.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object); err != nil {
				return client.IgnoreNotFound(err)
			}
			controllerutil.RemoveFinalizer(object, finalizerName)
			if err := k8sClient.Update(ctx, object); err != nil {
				return fmt.Errorf("failed to remove the finalizer '%s'. Error: %v", finalizerName, err)
			}
		}
		log.Info(fmt.Sprintf("removed finalizer %s from %s", finalizerName, object.GetControllerName()))
		return nil
	}
	return nil
}

func UpdateStatus(ctx context.Context, k8sClient client.Client, object common.SAPBTPResource) error {
	log := GetLogger(ctx)
	log.Info(fmt.Sprintf("updating %s status", object.GetObjectKind().GroupVersionKind().Kind))
	return k8sClient.Status().Update(ctx, object)
}

func ShouldIgnoreNonTransient(log logr.Logger, resource common.SAPBTPResource, timeout time.Duration) bool {
	annotations := resource.GetAnnotations()
	if len(annotations) == 0 || len(annotations[common.IgnoreNonTransientErrorAnnotation]) == 0 {
		return false
	}

	// we ignore the error
	// for service instances, the value is validated in webhook
	// for service bindings, the annotation is not allowed
	annotationTime, _ := time.Parse(time.RFC3339, annotations[common.IgnoreNonTransientErrorTimestampAnnotation])
	sinceAnnotation := time.Since(annotationTime)
	if sinceAnnotation > timeout {
		log.Info(fmt.Sprintf("timeout of %s reached - error is considered to be non transient. time passed since annotation timestamp %s", timeout, sinceAnnotation))
		return false
	}
	log.Info(fmt.Sprintf("timeout of %s was not reached - error is considered to be transient. ime passed since annotation timestamp %s", timeout, sinceAnnotation))
	return true
}

func NormalizeCredentials(credentialsJSON json.RawMessage) (map[string][]byte, []SecretMetadataProperty, error) {
	var credentialsMap map[string]interface{}
	err := json.Unmarshal(credentialsJSON, &credentialsMap)
	if err != nil {
		return nil, nil, err
	}

	normalized := make(map[string][]byte)
	metadata := make([]SecretMetadataProperty, 0)
	for propertyName, value := range credentialsMap {
		keyString := strings.Replace(propertyName, " ", "_", -1)
		normalizedValue, typpe, err := serialize(value)
		if err != nil {
			return nil, nil, err
		}
		metadata = append(metadata, SecretMetadataProperty{
			Name:   keyString,
			Format: string(typpe),
		})
		normalized[keyString] = normalizedValue
	}
	return normalized, metadata, nil
}

func BuildUserInfo(ctx context.Context, userInfo *v1.UserInfo) string {
	log := GetLogger(ctx)
	if userInfo == nil {
		return ""
	}
	userInfoStr, err := json.Marshal(userInfo)
	if err != nil {
		log.Error(err, "failed to prepare user info")
		return ""
	}

	return string(userInfoStr)
}

func SliceContains(slice []string, i string) bool {
	for _, s := range slice {
		if s == i {
			return true
		}
	}

	return false
}

func RandStringRunes(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func GetLogger(ctx context.Context) logr.Logger {
	return ctx.Value(LogKey{}).(logr.Logger)
}

func HandleError(ctx context.Context, k8sClient client.Client, operationType smClientTypes.OperationCategory, err error, resource common.SAPBTPResource, ignoreNonTransient bool) (ctrl.Result, error) {
	log := GetLogger(ctx)
	var smError *sm.ServiceManagerError
	ok := errors.As(err, &smError)
	if !ok {
		log.Info("unable to cast error to SM error, will be treated as non transient")
		return MarkAsNonTransientError(ctx, k8sClient, operationType, err.Error(), resource)
	}
	if ignoreNonTransient || IsTransientError(smError, log) {
		return MarkAsTransientError(ctx, k8sClient, operationType, smError, resource)
	}

	return MarkAsNonTransientError(ctx, k8sClient, operationType, smError.Error(), resource)
}

func IsTransientError(smError *sm.ServiceManagerError, log logr.Logger) bool {
	statusCode := smError.GetStatusCode()
	log.Info(fmt.Sprintf("SM returned error with status code %d", statusCode))
	return isTransientStatusCode(statusCode) || isConcurrentOperationError(smError)
}

func IsMarkedForDeletion(object metav1.ObjectMeta) bool {
	return !object.DeletionTimestamp.IsZero()
}

func RemoveAnnotations(ctx context.Context, k8sClient client.Client, object common.SAPBTPResource, keys ...string) error {
	log := GetLogger(ctx)
	annotations := object.GetAnnotations()
	shouldUpdate := false
	if annotations != nil {
		for _, key := range keys {
			if _, ok := annotations[key]; ok {
				log.Info(fmt.Sprintf("deleting annotation with key %s", key))
				delete(annotations, key)
				shouldUpdate = true
			}
		}
		if shouldUpdate {
			object.SetAnnotations(annotations)
			return k8sClient.Update(ctx, object)
		}
	}
	return nil
}

func isConcurrentOperationError(smError *sm.ServiceManagerError) bool {
	// service manager returns 422 for resources that have another operation in progress
	// in this case 422 status code is transient
	return smError.StatusCode == http.StatusUnprocessableEntity && smError.ErrorType == "ConcurrentOperationInProgress"
}

func isTransientStatusCode(StatusCode int) bool {
	return StatusCode == http.StatusTooManyRequests ||
		StatusCode == http.StatusServiceUnavailable ||
		StatusCode == http.StatusGatewayTimeout ||
		StatusCode == http.StatusBadGateway ||
		StatusCode == http.StatusNotFound
}

func serialize(value interface{}) ([]byte, format, error) {
	if byteArrayVal, ok := value.([]byte); ok {
		return byteArrayVal, JSON, nil
	}
	if strVal, ok := value.(string); ok {
		return []byte(strVal), TEXT, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, UNKNOWN, err
	}
	return data, JSON, nil
}
