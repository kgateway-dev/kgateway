package gloomtls

import (
	"net/http"

	. "github.com/onsi/gomega"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
)

var (
	expectedHealthyResponse = &testmatchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       ContainSubstring("Welcome to nginx!"),
	}
)
