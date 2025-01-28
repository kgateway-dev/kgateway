//go:build ignore

package gloomtls

import (
	"net/http"

	testmatchers "github.com/kgateway-dev/kgateway/test/gomega/matchers"
	. "github.com/onsi/gomega"
)

var (
	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       ContainSubstring("Welcome to nginx!"),
	}
)
