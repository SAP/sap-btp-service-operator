package log_utils

import (
	"context"

	"github.com/go-logr/logr"
)

type LogKey struct{}

func GetLogger(ctx context.Context) logr.Logger {
	return ctx.Value(LogKey{}).(logr.Logger)
}
