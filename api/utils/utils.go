package utils

import (
	"fmt"
	"time"

	"github.com/SAP/sap-btp-service-operator/api"
	"github.com/go-logr/logr"
)

func ValidateNonTransientTimestampAnnotation(log logr.Logger, resource api.SAPBTPResource) error {

	sinceAnnotation, exist, err := GetTimeSinceIgnoreNonTransientAnnotationTimestamp(log, resource)
	if err != nil {
		return err
	}
	if exist && sinceAnnotation < 0 {
		return fmt.Errorf("annotation %s cannot be a future timestamp", api.IgnoreNonTransientErrorTimestampAnnotation)
	}
	return nil
}

func IsIgnoreNonTransientAnnotationExistAndValid(log logr.Logger, resource api.SAPBTPResource, timeout time.Duration) bool {

	sinceAnnotation, exist, _ := GetTimeSinceIgnoreNonTransientAnnotationTimestamp(log, resource)
	if !exist {
		return false
	}
	if sinceAnnotation > timeout {
		log.Info(fmt.Sprintf("timeout reached- consider error to be non transient. since annotation timestamp %s, IgnoreNonTransientTimeout %s", sinceAnnotation, timeout))
		return false
	}
	log.Info(fmt.Sprintf("timeout didn't reached- consider error to be transient. since annotation timestamp %s, IgnoreNonTransientTimeout %s", sinceAnnotation, timeout))
	return true

}

func GetTimeSinceIgnoreNonTransientAnnotationTimestamp(log logr.Logger, resource api.SAPBTPResource) (time.Duration, bool, error) {
	annotation := resource.GetAnnotations()
	if annotation != nil {
		if _, ok := annotation[api.IgnoreNonTransientErrorAnnotation]; ok {
			log.Info("ignoreNonTransientErrorAnnotation annotation exist- checking timeout")
			annotationTime, err := time.Parse(time.RFC3339, annotation[api.IgnoreNonTransientErrorTimestampAnnotation])
			if err != nil {
				log.Error(err, fmt.Sprintf("failed to parse %s", api.IgnoreNonTransientErrorTimestampAnnotation))
				return time.Since(time.Now()), false, fmt.Errorf("annotation %s is not a valid timestamp", api.IgnoreNonTransientErrorTimestampAnnotation)
			}
			return time.Since(annotationTime), true, nil
		}
	}
	return time.Since(time.Now()), false, nil
}
