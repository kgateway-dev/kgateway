package utils

import (
	"context"
)

// StartFunc is a function that will be called when the kgateway process runs
type StartFunc func(ctx context.Context) error
