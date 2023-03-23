package assertions

import (
	"fmt"
	"net/http"

	"github.com/solo-io/gloo/test/testutils"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/go-utils/stats"
	"go.uber.org/zap/zapcore"
)

// LogLevelAssertion returns an Assertion to verify that the dynamic log level matches the provided value
func LogLevelAssertion(logLevel zapcore.Level) types.AsyncAssertion {
	loggingRequest := testutils.DefaultRequestBuilder().
		WithPort(stats.DefaultPort).
		WithPath("logging").
		Build()

	return Eventually(func(g Gomega) {
		g.Expect(http.DefaultClient.Do(loggingRequest)).Should(matchers.HaveHttpResponse(&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       fmt.Sprintf("{\"level\":\"%s\"}\n", logLevel.String()),
		}))
	}, "5s", ".1s")
}
