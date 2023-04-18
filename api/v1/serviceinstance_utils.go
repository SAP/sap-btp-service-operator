package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func ShouldHandleSharing(newShareState *bool, oldShareState metav1.ConditionStatus) bool {
	if newShareState == nil {
		return oldShareState == metav1.ConditionTrue
	}
	if *newShareState && oldShareState != metav1.ConditionTrue {
		return true
	}
	if !(*newShareState) && oldShareState != metav1.ConditionFalse {
		return true
	}
	return false
}
