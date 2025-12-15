package log_utils //nolint:stylecheck

import (
	"context"

	"github.com/go-logr/logr"
)

type logKey struct{}

var LogKey = logKey{}

func GetLogger(ctx context.Context) logr.Logger {
	v := ctx.Value(LogKey)
	if v == nil {
		return logr.Discard()
	}
	return v.(logr.Logger)
}
