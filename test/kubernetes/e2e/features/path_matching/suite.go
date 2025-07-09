package path_matching

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// BeforeTest runs before each test in the suite
func (s *testingSuite) BeforeTest(suiteName, testName string) {
	s.BaseTestingSuite.BeforeTest(suiteName, testName)

	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(s.Ctx, "httpbin", "httpbin", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
}

// TestExactMatch tests an HTTPRoute with a path match of type Exact
func (s *testingSuite) TestExactMatch() {
	// expected path works
	s.assertStatus("anything/justme", http.StatusOK)

	// all other paths do not
	s.assertStatus("anything/nope", http.StatusNotFound)

	s.assertStatus("anything/justmea", http.StatusNotFound)
	s.assertStatus("anything/justm", http.StatusNotFound)
	s.assertStatus("anything/justma", http.StatusNotFound)
	s.assertStatus("anything/ajustme", http.StatusNotFound)
	s.assertStatus("anything/ustme", http.StatusNotFound)
	s.assertStatus("anything/austme", http.StatusNotFound)

	s.assertStatus("anything/justme/", http.StatusNotFound)
	s.assertStatus("anything/justme/nope", http.StatusNotFound)
	s.assertStatus("anything/nope/justme", http.StatusNotFound)
}

// TestPrefixMatch tests an HTTPRoute with a path match of type PathPrefix
func (s *testingSuite) TestPrefixMatch() {
	// prefix path works
	s.assertStatus("anything/pre", http.StatusOK)

	// additional characters including or after a slash work
	s.assertStatus("anything/pre/", http.StatusOK)
	s.assertStatus("anything/pre/plus", http.StatusOK)
	s.assertStatus("anything/pre/plus/more", http.StatusOK)

	// all other paths do not
	s.assertStatus("anything/nope", http.StatusNotFound)

	s.assertStatus("anything/prea", http.StatusNotFound)
	s.assertStatus("anything/pr", http.StatusNotFound)
	s.assertStatus("anything/pra", http.StatusNotFound)
	s.assertStatus("anything/apre", http.StatusNotFound)
	s.assertStatus("anything/re", http.StatusNotFound)
	s.assertStatus("anything/are", http.StatusNotFound)

	s.assertStatus("anything/nope/pre", http.StatusNotFound)
}

// TestRegexMatch tests an HTTPRoute with a path match of type RegularExpression
// regex: /anything/plus/(this|that)/[^/]+?/\d[.]/(1.)/(.+)/end
func (s *testingSuite) TestRegexMatch() {
	// paths matching regex work
	s.assertStatus("anything/plus/this/what3v3r/4./1a/stuff/end", http.StatusOK)
	s.assertStatus("anything/plus/that/what3v3r/4./1a/stuff/end", http.StatusOK)
	s.assertStatus("anything/plus/this/!@$*&().a-b_c~'+4,;0=:sdf/4./1a/stuff/end", http.StatusOK) // unusual chars
	s.assertStatus("anything/plus/this/what3v3r/0./1a/stuff/end", http.StatusOK)
	s.assertStatus("anything/plus/this/what3v3r/4./15/stuff/end", http.StatusOK)
	s.assertStatus("anything/plus/this/what3v3r/4./1a/plus/more/stuff/end", http.StatusOK) // additional path elements where permitted

	// all other paths do not
	s.assertStatus("anything/this/what3v3r/4./1a/stuff/end", http.StatusNotFound)     // missing early path element
	s.assertStatus("anything/plus/this/what3v3r4./1a/stuff/end", http.StatusNotFound) // merging path elements
	s.assertStatus("anything/plus/this/what3v3r/4.1a/stuff/end", http.StatusNotFound) // merging path elements
	s.assertStatus("anything/plus/this/what3v3r/4./1astuff/end", http.StatusNotFound) // merging path elements
	s.assertStatus("anything/plus/this/what3v3r/4./1a/stuffend", http.StatusNotFound) // merging path elements

	s.assertStatus("anything/plus/thus/what3v3r/4./1a/stuff/end", http.StatusNotFound)        // not this or that
	s.assertStatus("anything/plus/this|that/what3v3r/4./1a/stuff/end", http.StatusNotFound)   // literal this or that
	s.assertStatus("anything/plus/(this|that)/what3v3r/4./1a/stuff/end", http.StatusNotFound) // literal this or that
	s.assertStatus("anything/plus//what3v3r/4./1a/stuff/end", http.StatusNotFound)            // missing this or that
	s.assertStatus("anything/plus/what3v3r/4./1a/stuff/end", http.StatusNotFound)             // missing this or that

	s.assertStatus("anything/plus/this/what/3v3r/4./1a/stuff/end", http.StatusNotFound) // 2 path elements where 1 is expected
	s.assertStatus("anything/plus/this//4./1a/stuff/end", http.StatusNotFound)          // 0 path elements where 1 is expected
	s.assertStatus("anything/plus/this/4./1a/stuff/end", http.StatusNotFound)           // 0 path elements where 1 is expected

	s.assertStatus("anything/plus/this/what3v3r/12./1a/stuff/end", http.StatusNotFound) // 2 digits where 1 is expected
	s.assertStatus("anything/plus/this/what3v3r/./1a/stuff/end", http.StatusNotFound)   // 0 digits where 1 is expected
	s.assertStatus("anything/plus/this/what3v3r/a./1a/stuff/end", http.StatusNotFound)  // letter where digit is expected
	s.assertStatus("anything/plus/this/what3v3r/4../1a/stuff/end", http.StatusNotFound) // 2 dots where 1 is expected
	s.assertStatus("anything/plus/this/what3v3r/4/1a/stuff/end", http.StatusNotFound)   // 0 dots where 1 is expected
	s.assertStatus("anything/plus/this/what3v3r/4a/1a/stuff/end", http.StatusNotFound)  // letter where dot is expected
	s.assertStatus("anything/plus/this/what3v3r/a4./1a/stuff/end", http.StatusNotFound) // extra char
	s.assertStatus("anything/plus/this/what3v3r/4.a/1a/stuff/end", http.StatusNotFound) // extra char

	s.assertStatus("anything/plus/this/what3v3r/4./11a/stuff/end", http.StatusNotFound) // 2 '1's where 1 is expected
	s.assertStatus("anything/plus/this/what3v3r/4./a/stuff/end", http.StatusNotFound)   // 0 '1's where 1 is expected
	s.assertStatus("anything/plus/this/what3v3r/4./0a/stuff/end", http.StatusNotFound)  // 0 where '1' is expected
	s.assertStatus("anything/plus/this/what3v3r/4./1/stuff/end", http.StatusNotFound)   // missing char
	s.assertStatus("anything/plus/this/what3v3r/4./1ab/stuff/end", http.StatusNotFound) // extra char

	s.assertStatus("anything/plus/this/what3v3r/4./1a//end", http.StatusNotFound) // missing path element content where expected
	s.assertStatus("anything/plus/this/what3v3r/4./1a/end", http.StatusNotFound)  // missing path element where expected

	s.assertStatus("anything/plus/this/what3v3r/4./1a/stuff/en", http.StatusNotFound)       // missing char
	s.assertStatus("anything/plus/this/what3v3r/4./1a/stuff/nd", http.StatusNotFound)       // missing char
	s.assertStatus("anything/plus/this/what3v3r/4./1a/stuff/ends", http.StatusNotFound)     // extra char
	s.assertStatus("anything/plus/this/what3v3r/4./1a/stuff/end/", http.StatusNotFound)     // extra slash
	s.assertStatus("anything/plus/this/what3v3r/4./1a/stuff/end/more", http.StatusNotFound) // extra path element
}

// TestPrefixRewrite tests an HTTPRoute with a path match of type PathPrefix
// which also uses the URLRewrite filter to drop that prefix
func (s *testingSuite) TestPrefixRewrite() {
	// paths matching prefix drop it and work
	s.assertStatus("el360/bi/v1/anything", http.StatusOK)
	s.assertStatus("el360/bi/v1/status/200", http.StatusOK)
	s.assertStatus("el360/bi/v1/status/418", http.StatusTeapot)

	// all other paths do not work
	s.assertStatus("el360/bi/v1anything/whatever", http.StatusNotFound)
	s.assertStatus("el360/bi/anything/whatever", http.StatusNotFound)
	s.assertStatus("l360/bi/v1/anything/whatever", http.StatusNotFound)
	s.assertStatus("bi/v1/anything/whatever", http.StatusNotFound)
	s.assertStatus("anything/whatever", http.StatusNotFound)
	s.assertStatus("status/200", http.StatusNotFound)
	s.assertStatus("anything/el360/bi/v1/anything/whatever", http.StatusNotFound)
}

func (s *testingSuite) assertStatus(path string, status int) {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath(path),
		},
		&matchers.HttpResponse{
			StatusCode: status,
		},
	)
}
