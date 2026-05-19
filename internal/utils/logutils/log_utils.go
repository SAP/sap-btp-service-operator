package logutils

import (
	"context"

	"github.com/go-logr/logr"
)

type logKey struct{}
type correlationIDKey struct{}

var LogKey = logKey{}
var CorrelationIDKey = correlationIDKey{}

func GetLogger(ctx context.Context) logr.Logger {
	v := ctx.Value(LogKey)
	if v == nil {
		return logr.Discard()
	}
	return v.(logr.Logger)
}

func GetCorrelationID(ctx context.Context) string {
	v := ctx.Value(CorrelationIDKey)
	if v == nil {
		return ""
	}
	return v.(string)
}
