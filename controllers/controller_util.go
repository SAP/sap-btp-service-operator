package controllers

import (
	"context"
	"encoding/json"
	"strings"

	v1 "k8s.io/api/authentication/v1"
)

func normalizeCredentials(credentialsJSON json.RawMessage) (map[string][]byte, error) {
	var credentialsMap map[string]interface{}
	err := json.Unmarshal(credentialsJSON, &credentialsMap)
	if err != nil {
		return nil, err
	}

	normalized := make(map[string][]byte)
	for propertyName, value := range credentialsMap {
		keyString := strings.Replace(propertyName, " ", "_", -1)
		normalizedValue, err := serialize(value)
		if err != nil {
			return nil, err
		}
		normalized[keyString] = normalizedValue
	}
	return normalized, nil
}

func buildUserInfo(ctx context.Context, userInfo *v1.UserInfo) string {
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
