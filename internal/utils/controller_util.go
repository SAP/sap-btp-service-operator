package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

func RemoveFinalizer(ctx context.Context, k8sClient client.Client, object client.Object, finalizerName string, controllerName common.ControllerName) error {
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
		log.Info(fmt.Sprintf("removed finalizer %s from %s", finalizerName, controllerName))
		return nil
	}
	return nil
}

func UpdateStatus(ctx context.Context, k8sClient client.Client, object common.SAPBTPResource) error {
	log := GetLogger(ctx)
	log.Info(fmt.Sprintf("updating %s status", object.GetObjectKind().GroupVersionKind().Kind))
	return k8sClient.Status().Update(ctx, object)
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

func HandleError(ctx context.Context, k8sClient client.Client, operationType smClientTypes.OperationCategory, err error, resource common.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	var smError *sm.ServiceManagerError
	if ok := errors.As(err, &smError); ok {
		if smError.StatusCode == http.StatusTooManyRequests {
			log.Info(fmt.Sprintf("SM returned 429 (%s), requeueing...", smError.Error()))
			return handleRateLimitError(smError, log)
		}

		log.Info(fmt.Sprintf("SM returned error: %s", smError.Error()))
		return MarkAsTransientError(ctx, k8sClient, operationType, smError, resource)
	}

	log.Info(fmt.Sprintf("unable to cast error to SM error, will be treated as non transient. (error: %v)", err))
	return MarkAsNonTransientError(ctx, k8sClient, operationType, err, resource)
}

// ParseNamespacedName converts a "namespace/name" string to a types.NamespacedName object.
func ParseNamespacedName(input string) (apimachinerytypes.NamespacedName, error) {
	parts := strings.SplitN(input, "/", 2)
	if len(parts) != 2 {
		return apimachinerytypes.NamespacedName{}, fmt.Errorf("invalid format: expected 'namespace/name', got '%s'", input)
	}
	return apimachinerytypes.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
}

func handleRateLimitError(smError *sm.ServiceManagerError, log logr.Logger) (ctrl.Result, error) {
	retryAfterStr := smError.ResponseHeaders.Get("Retry-After")
	if len(retryAfterStr) > 0 {
		log.Info(fmt.Sprintf("SM returned 429 with Retry-After: %s, requeueing after it...", retryAfterStr))
		retryAfter, err := time.Parse(time.DateTime, retryAfterStr[:len(time.DateTime)]) // format 2024-11-11 14:59:33 +0000 UTC
		if err != nil {
			log.Error(err, "failed to parse Retry-After header, using default requeue time")
		} else {
			timeToRequeue := time.Until(retryAfter)
			log.Info(fmt.Sprintf("requeueing after %d minutes, %d seconds", int(timeToRequeue.Minutes()), int(timeToRequeue.Seconds())%60))
			return ctrl.Result{RequeueAfter: timeToRequeue}, nil
		}
	}

	return ctrl.Result{Requeue: true}, nil
}

func HandleDeleteError(ctx context.Context, k8sClient client.Client, err error, object common.SAPBTPResource) (ctrl.Result, error) {
	log := GetLogger(ctx)
	log.Info(fmt.Sprintf("handling delete error: %v", err))
	var smError *sm.ServiceManagerError
	if errors.As(err, &smError) && smError.StatusCode == http.StatusTooManyRequests {
		return handleRateLimitError(smError, log)
	}

	if _, updateErr := MarkAsNonTransientError(ctx, k8sClient, smClientTypes.DELETE, err, object); updateErr != nil {
		log.Error(updateErr, "failed to update resource status")
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{}, err
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

func AddWatchForSecret(ctx context.Context, k8sClient client.Client, secret *corev1.Secret, instanceUID string) error {
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations[common.WatchSecretAnnotation+instanceUID] = "true"
	controllerutil.AddFinalizer(secret, common.FinalizerName)

	return k8sClient.Update(ctx, secret)
}

func RemoveWatchForSecret(ctx context.Context, k8sClient client.Client, secretKey apimachinerytypes.NamespacedName, instanceUID string) error {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, secretKey, secret); err != nil {
		return err
	}
	delete(secret.Annotations, common.WatchSecretAnnotation+instanceUID)
	if !IsSecretWatched(secret.Annotations) {
		controllerutil.RemoveFinalizer(secret, common.FinalizerName)
	}

	return k8sClient.Update(ctx, secret)
}

func IsSecretWatched(secretAnnotations map[string]string) bool {
	for key := range secretAnnotations {
		if strings.HasPrefix(common.WatchSecretAnnotation, key) {
			return true
		}
	}
	return false
}
