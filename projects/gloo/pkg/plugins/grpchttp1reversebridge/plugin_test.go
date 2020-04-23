package grpchttp1reversebridge_test

import (
	"testing"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpchttp1reversebridge"
)

func TestNewPlugin(t *testing.T) {
	_ = grpchttp1reversebridge.NewPlugin()
}
