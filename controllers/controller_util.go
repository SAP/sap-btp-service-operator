package controllers

import (
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/authentication/v1"
)

func normalizeCredentials(credentialsJSON json.RawMessage, flattenLevel int64) (map[string][]byte, error) {
	var credentialsMap map[string]interface{}
	err := json.Unmarshal(credentialsJSON, &credentialsMap)
	if err != nil {
		return nil, err
	}

	normalized := make(map[string][]byte)
	return flattenCredentials(credentialsMap, normalized, "", 1, flattenLevel)
}

func flattenCredentials(currentObject map[string]interface{}, normalized map[string][]byte, prefix string, currentLevel int64, flattenLevel int64) (map[string][]byte, error) {
	for propertyName, value := range currentObject {
		keyString := prefix + strings.Replace(propertyName, " ", "_", -1)

		subObject, ok := value.(map[string]interface{})
		if ok && currentLevel < flattenLevel {
			_, err := flattenCredentials(subObject, normalized, keyString+"_", currentLevel+1, flattenLevel)
			if err != nil {
				return nil, err
			}
		} else {
			normalizedValue, err := serialize(value)
			if err != nil {
				return nil, err
			}
			normalized[keyString] = normalizedValue
		}
	}
	return normalized, nil
}

func buildUserInfo(userInfo *v1.UserInfo, log logr.Logger) string {
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

func serialize(value interface{}) ([]byte, error) {
	if byteArrayVal, ok := value.([]byte); ok {
		return byteArrayVal, nil
	}
	if strVal, ok := value.(string); ok {
		return []byte(strVal), nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func contains(slice []string, i string) bool {
	for _, s := range slice {
		if s == i {
			return true
		}
	}

	return false
}
