package plugins_test

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/testutils"
)

func TestInferencePoolStatus(t *testing.T) {
	testutils.RunForDirectory(t, "testdata/inferencepool", func(t *testing.T, ctx plugins.PolicyCtx) (any, []any) {
		sq, _ := testutils.Syncer(t, ctx, "InferencePool")
		return sq.Dump(), []any{}
	})
}
