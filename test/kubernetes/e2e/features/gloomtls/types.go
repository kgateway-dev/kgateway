//go:build ignore

package gloomtls

import (
	"net/http"
	"path/filepath"

	testmatchers "github.com/kgateway-dev/kgateway/test/gomega/matchers"
	. "github.com/onsi/gomega"
	"github.com/solo-io/skv2/codegen/util"
)

var (
	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       ContainSubstring("Welcome to nginx!"),
	}

	edgeRoutingResources = filepath.Join(util.MustGetThisDir(), "testdata", "edge_resources.yaml")
)
