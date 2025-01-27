//go:build ignore

package gloomtls

import (
	"net/http"
	"path/filepath"

	. "github.com/onsi/gomega"
	testmatchers "github.com/kgateway-dev/kgateway/test/gomega/matchers"
	"github.com/solo-io/skv2/codegen/util"
)

var (
	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       ContainSubstring("Welcome to nginx!"),
	}

	edgeRoutingResources = filepath.Join(util.MustGetThisDir(), "testdata", "edge_resources.yaml")
)
