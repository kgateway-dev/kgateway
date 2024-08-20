package helper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"

	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/types"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/log"
)

const (
	defaultCurlImage = "curlimages/curl:7.83.1"
	CurlName         = "curl"
	CurlPort         = 3000
)

func NewCurl(namespace string) (TestContainer, error) {
	return newTestContainer(namespace, defaultCurlImage, CurlName, CurlPort, false, []string{"tail", "-f", "/dev/null"})
}

type CurlOpts struct {
	Protocol          string
	Path              string
	Method            string
	Host              string
	Service           string
	CaFile            string
	Body              string
	Headers           map[string]string
	Port              int
	ReturnHeaders     bool
	ConnectionTimeout int

	// Verbose is used to configure the verbosity of the curl request
	// Deprecated: see buildCurlArgs() for details about why we always default this to true
	Verbose       bool
	LogResponses  bool
	AllowInsecure bool
	// WithoutStats sets the -s flag to prevent download stats from printing
	WithoutStats bool
	// Optional SNI name to resolve domain to when sending request
	Sni        string
	SelfSigned bool

	// Retries on Curl requests are disabled by default because they historically were not configurable
	// Curls to a remote container may be subject to network flakes and therefore using retries
	// can be a useful mechanism to avoid test flakes
	Retries struct {
		Retry        int
		RetryDelay   int
		RetryMaxTime int
	}
}

var (
	ErrCannotCurl = errors.New("cannot curl")
	errCannotCurl = func(imageName, imageTag string) error {
		return errors.Wrapf(ErrCannotCurl, "testContainer from image %s:%s", imageName, imageTag)
	}
	DefaultCurlTimeout        = time.Second * 20 // DefaultCurlTimeout is the default timeout for "Eventually" curl assertions
	DefaultCurlPollingTimeout = time.Second * 2  // DefaultCurlPollingTimeout is the default pollinginterval for "Eventually" curl assertions
)

var getTimeoutsAsInterfaces = helpers.GetDefaultTimingsTransform(DefaultCurlTimeout, DefaultCurlPollingTimeout)

func GetTimeouts(timeout ...time.Duration) (currentTimeout, pollingInterval time.Duration) {
	// Convert the timeouts to interface{}s
	interfaceTimeouts := make([]interface{}, len(timeout))
	for i, t := range timeout {
		interfaceTimeouts[i] = t
	}

	timeoutAny, pollingIntervalAny := getTimeoutsAsInterfaces(interfaceTimeouts...)
	currentTimeout = timeoutAny.(time.Duration)
	pollingInterval = pollingIntervalAny.(time.Duration)
	return currentTimeout, pollingInterval
}

func (t *testContainer) CurlEventuallyShouldOutput(opts CurlOpts, expectedOutput interface{}, ginkgoOffset int, timeout ...time.Duration) {
	t.CurlEventuallyShouldRespond(opts, expectedOutput, ginkgoOffset, timeout...)
}

func (t *testContainer) CurlEventuallyShouldRespond(opts CurlOpts, expectedResponse interface{}, ginkgoOffset int, timeout ...time.Duration) {
	currentTimeout, pollingInterval := GetTimeouts(timeout...)
	// for some useful-ish output
	tick := time.Tick(currentTimeout / 8)

	cli := kubectl.NewCli()

	EventuallyWithOffset(ginkgoOffset+1, func(g Gomega) {
		curlResp, err := cli.CurlFromPod(context.Background(), kubectl.PodExecOptions{Name: t.podName, Namespace: t.namespace, Container: t.podName}, t.buildCurlOptions(opts)...)
		if err != nil {
			// trigger an early exit if the pod has been deleted.
			if strings.Contains(err.Error(), `pods "testserver" not found`) {
				ginkgo.Fail(err.Error())
			}

			// Will always fail
			g.Expect(err).NotTo(HaveOccurred())
		}
		select {
		default:
			break
		case <-tick:
			if opts.LogResponses {
				log.GreyPrintf("running: %v\nwant %v\nhave: %+v", opts, expectedResponse, curlResp)
			}
		}

		expectedResponseMatcher := GetExpectedResponseMatcher(expectedResponse)
		g.Expect(curlResp).To(expectedResponseMatcher)
		if opts.LogResponses {
			log.GreyPrintf("success: \n%s\n%s\n", curlResp.StdErr, curlResp.StdOut)
		}
	}, currentTimeout, pollingInterval).Should(Succeed())
}

// GetExpectedResponseMatcher takes an interface and converts it into the types.GomegaMatcher
// that will be used to assert that a given Curl response, matches an expected shape
func GetExpectedResponseMatcher(expectedOutput interface{}) types.GomegaMatcher {
	switch a := expectedOutput.(type) {
	case string:
		// In the past, this Curl utility accepted a string, and only asserted that the http response body
		// contained that as a substring.
		// To ensure that all tests which relied on this functionality still work, we accept a string, but
		// improve the assertion to also validate that the StatusCode was a 200
		return WithTransform(transforms.WithCurlResponse, matchers.HaveHttpResponse(&matchers.HttpResponse{
			Body:       ContainSubstring(a),
			StatusCode: http.StatusOK,
		}))
	case *matchers.HttpResponse:
		// There are some cases in tests where we require asserting that a response was not a 200
		// To support that case, we allow developers to supply an HttpResponse object, which defines the
		// expected response. See matchers.HaveHttpResponse for more details
		// This is actually the preferred input, as it means that the WithCurlHttpResponse transform
		// can process the Curl response
		return WithTransform(transforms.WithCurlResponse, matchers.HaveHttpResponse(a))
	case types.GomegaMatcher:
		// As a fallback, we also allow developers to define the expected matcher explicitly
		// If this is necessary, it means that the WithCurlHttpResponse transform likely needs to
		// be expanded. In that case, we should seriously consider expanding that functionality
		return a
	default:
		ginkgo.Fail(fmt.Sprintf("Invalid expectedOutput: %+v", expectedOutput))
	}
	return nil
}

func (t *testContainer) buildCurlOptions(opts CurlOpts) []curl.Option {
	var curlOptions []curl.Option

	appendOption := func(option curl.Option) {
		curlOptions = append(curlOptions, option)
	}

	// The testContainer relies on the transforms.WithCurlHttpResponse to validate the response is what
	// we would expect
	// For this transform to behave appropriately, we must execute the request with verbose=true
	appendOption(curl.VerboseOutput())

	if opts.Retries.Retry != 0 {
		appendOption(curl.WithRetries(opts.Retries.Retry, opts.Retries.RetryDelay, opts.Retries.RetryMaxTime))
		appendOption(curl.WithRetryConnectionRefused(true))
	}

	if opts.WithoutStats {
		appendOption(curl.Silent())
	}
	if opts.ReturnHeaders {
		appendOption(curl.WithHeadersOnly())
	}

	appendOption(curl.WithConnectionTimeout(opts.ConnectionTimeout))
	appendOption(curl.WithPath(opts.Path))

	if opts.Method != "" {
		appendOption(curl.WithMethod(opts.Method))
	}
	if opts.CaFile != "" {
		appendOption(curl.WithCaFile(opts.CaFile))
	}
	if opts.Host != "" {
		appendOption(curl.WithHostHeader(opts.Host))
	}
	if opts.Body != "" {
		appendOption(curl.WithPostBody(opts.Body))
	}
	for h, v := range opts.Headers {
		appendOption(curl.WithHeader(h, v))
	}
	if opts.AllowInsecure {
		appendOption(curl.IgnoreServerCert())
	}

	port := opts.Port
	if port == 0 {
		port = 8080
	}
	appendOption(curl.WithPort(port))

	if opts.Protocol != "" {
		appendOption(curl.WithScheme(opts.Protocol))
	}

	host := opts.Service
	if host == "" {
		host = "test-ingress"
	}
	appendOption(curl.WithHost(host))

	if opts.SelfSigned {
		appendOption(curl.IgnoreServerCert())
	}
	if opts.Sni != "" {
		appendOption(curl.WithSni(opts.Sni))
	}

	return curlOptions
}

func (t *testContainer) buildCurlArgs(opts CurlOpts) []string {
	curlOptions := t.buildCurlOptions(opts)

	args := append([]string{"curl"}, curl.BuildArgs(curlOptions...)...)
	log.Printf("running: %v", strings.Join(args, " "))

	return args
}

func (t *testContainer) Curl(opts CurlOpts) (string, error) {
	if !t.CanCurl() {
		return "", errCannotCurl(t.containerImageName, t.imageTag)
	}

	args := t.buildCurlArgs(opts)
	return t.Exec(args...)
}

func (t *testContainer) CurlAsync(opts CurlOpts) (io.Reader, chan struct{}, error) {
	if !t.CanCurl() {
		return nil, nil, errCannotCurl(t.containerImageName, t.imageTag)
	}

	args := t.buildCurlArgs(opts)
	return t.ExecAsync(args...)
}

func (t *testContainer) CurlAsyncChan(opts CurlOpts) (<-chan io.Reader, chan struct{}, error) {
	if !t.CanCurl() {
		return nil, nil, errCannotCurl(t.containerImageName, t.imageTag)
	}

	args := t.buildCurlArgs(opts)
	return t.ExecChan(&bytes.Buffer{}, args...)
}
