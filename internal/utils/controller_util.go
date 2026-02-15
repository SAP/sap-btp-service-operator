package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SAP/sap-btp-service-operator/internal/utils/logutils"
	corev1 "k8s.io/api/core/v1"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

func RemoveFinalizer(ctx context.Context, k8sClient client.Client, object client.Object, finalizerName string) error {
	log := logutils.GetLogger(ctx)
	if controllerutil.RemoveFinalizer(object, finalizerName) {
		log.Info(fmt.Sprintf("removing finalizer %s from resource %s named '%s' in namespace '%s'", finalizerName, object.GetObjectKind(), object.GetName(), object.GetNamespace()))
		return k8sClient.Update(ctx, object)
	}
	return nil
}

func UpdateStatus(ctx context.Context, k8sClient client.Client, object common.SAPBTPResource) error {
	log := logutils.GetLogger(ctx)
	log.Info(fmt.Sprintf("updating %s status", object.GetObjectKind().GroupVersionKind().Kind))
	object.SetObservedGeneration(getLastObservedGen(object))
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
	log := logutils.GetLogger(ctx)
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

func HandleServiceManagerError(ctx context.Context, k8sClient client.Client, resource common.SAPBTPResource, operationType smClientTypes.OperationCategory, err error) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	var smError *sm.ServiceManagerError
	if ok := errors.As(err, &smError); ok {
		if smError.StatusCode == http.StatusTooManyRequests {
			log.Info(fmt.Sprintf("SM returned 429 (%s), requeueing...", smError.Error()))
			return handleRateLimitError(ctx, k8sClient, resource, operationType, smError)
		}
	}

	return HandleOperationFailure(ctx, k8sClient, resource, operationType, err)
}

func HandleCredRotationError(ctx context.Context, k8sClient client.Client, binding common.SAPBTPResource, err error) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	var smError *sm.ServiceManagerError
	if ok := errors.As(err, &smError); ok {
		if smError.StatusCode == http.StatusTooManyRequests {
			log.Info(fmt.Sprintf("SM returned 429 (%s), requeueing...", smError.Error()))
			return handleRateLimitError(ctx, k8sClient, binding, common.Unknown, smError)
		}
		log.Info(fmt.Sprintf("SM returned error: %s", smError.Error()))
	}

	log.Info("updating cred rotation condition with error", err)
	SetCredRotationInProgressConditions(common.CredPreparing, err.Error(), binding)
	return ctrl.Result{}, UpdateStatus(ctx, k8sClient, binding)
}

// ParseNamespacedName converts a "namespace/name" string to a types.NamespacedName object.
func ParseNamespacedName(input string) (apimachinerytypes.NamespacedName, error) {
	parts := strings.SplitN(input, "/", 2)
	if len(parts) != 2 {
		return apimachinerytypes.NamespacedName{}, fmt.Errorf("invalid format: expected 'namespace/name', got '%s'", input)
	}
	return apimachinerytypes.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
}

func IsMarkedForDeletion(object metav1.ObjectMeta) bool {
	return !object.DeletionTimestamp.IsZero()
}

func RemoveAnnotations(ctx context.Context, k8sClient client.Client, object common.SAPBTPResource, keys ...string) error {
	log := logutils.GetLogger(ctx)
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

func AddWatchForSecretIfNeeded(ctx context.Context, k8sClient client.Client, secret *corev1.Secret, instanceUID string) error {
	log := logutils.GetLogger(ctx)
	updateRequired := false
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	if len(secret.Annotations[common.WatchSecretAnnotation+string(instanceUID)]) == 0 {
		log.Info(fmt.Sprintf("adding secret watch annotation for instance %s on secret %s", instanceUID, secret.Name))
		secret.Annotations[common.WatchSecretAnnotation+instanceUID] = "true"
		updateRequired = true
	}
	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}
	if secret.Labels[common.WatchSecretLabel] != "true" {
		log.Info(fmt.Sprintf("adding watch label for secret %s", secret.Name))
		secret.Labels[common.WatchSecretLabel] = "true"
		controllerutil.AddFinalizer(secret, common.FinalizerName)
		updateRequired = true
	}
	if updateRequired {
		return k8sClient.Update(ctx, secret)
	}

	return nil
}

func RemoveWatchForSecret(ctx context.Context, k8sClient client.Client, secretKey apimachinerytypes.NamespacedName, instanceUID string) error {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, secretKey, secret); err != nil {
		return client.IgnoreNotFound(err)
	}

	delete(secret.Annotations, common.WatchSecretAnnotation+instanceUID)
	if !IsSecretWatched(secret.Annotations) {
		delete(secret.Labels, common.WatchSecretLabel)
		controllerutil.RemoveFinalizer(secret, common.FinalizerName)
	}
	return k8sClient.Update(ctx, secret)
}

func IsSecretWatched(secretLabels map[string]string) bool {
	return secretLabels != nil && secretLabels[common.WatchSecretLabel] == "true"
}

func GetLabelKeyForInstanceSecret(secretName string) string {
	return common.InstanceSecretRefLabel + secretName
}

func HandleInstanceSharingError(ctx context.Context, k8sClient client.Client, object common.SAPBTPResource, status metav1.ConditionStatus, reason string, err error) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)

	errMsg := err.Error()
	if smError, ok := err.(*sm.ServiceManagerError); ok {
		log.Info(fmt.Sprintf("SM returned error with status code %d", smError.StatusCode))
		errMsg = smError.Error()

		if smError.StatusCode == http.StatusTooManyRequests {
			return handleRateLimitError(ctx, k8sClient, object, common.Unknown, smError)
		} else if reason == common.ShareFailed &&
			(smError.StatusCode == http.StatusBadRequest || smError.StatusCode == http.StatusInternalServerError) {
			/* non-transient error may occur only when sharing
			   SM return 400 when plan is not sharable
			   SM returns 500 when TOGGLES_ENABLE_INSTANCE_SHARE_FROM_OPERATOR feature toggle is off */
			reason = common.ShareNotSupported
		}
	}

	SetSharedCondition(object, status, reason, errMsg)
	if updateErr := UpdateStatus(ctx, k8sClient, object); updateErr != nil {
		log.Error(updateErr, "failed to update instance status")
		return ctrl.Result{}, updateErr
	}

	return ctrl.Result{}, err
}

func handleRateLimitError(ctx context.Context, sClient client.Client, resource common.SAPBTPResource, operationType smClientTypes.OperationCategory, smError *sm.ServiceManagerError) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	SetInProgressConditions(ctx, operationType, "", resource, false)
	if updateErr := UpdateStatus(ctx, sClient, resource); updateErr != nil {
		log.Info("failed to update status after rate limit error")
		return ctrl.Result{}, updateErr
	}

	retryAfterStr := smError.ResponseHeaders.Get("Retry-After")
	if len(retryAfterStr) > 0 {
		log.Info(fmt.Sprintf("SM returned 429 with Retry-After: %s, requeueing after it...", retryAfterStr))
		if retryAfter, err := time.Parse(time.DateTime, retryAfterStr[:len(time.DateTime)]); err != nil { // format 2024-11-11 14:59:33 +0000 UTC
			log.Error(err, "failed to parse Retry-After header, using default requeue time")
		} else {
			timeToRequeue := time.Until(retryAfter)
			log.Info(fmt.Sprintf("requeueing after %d minutes, %d seconds", int(timeToRequeue.Minutes()), int(timeToRequeue.Seconds())%60))
			return ctrl.Result{RequeueAfter: timeToRequeue}, nil
		}
	}

	return ctrl.Result{}, smError
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
