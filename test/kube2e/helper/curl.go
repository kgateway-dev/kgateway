package helper

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/types"

	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/solo-io/gloo/test/testutils"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/log"
)

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
	Verbose           bool
	LogResponses      bool
	AllowInsecure     bool
	// WithoutStats sets the -s flag to prevent download stats from printing
	WithoutStats bool
	// Optional SNI name to resolve domain to when sending request
	Sni        string
	SelfSigned bool
}

var (
	ErrCannotCurl = errors.New("cannot curl")
	errCannotCurl = func(imageName, imageTag string) error {
		return errors.Wrapf(ErrCannotCurl, "testContainer from image %s:%s", imageName, imageTag)
	}
)

func getTimeouts(timeout ...time.Duration) (currentTimeout, pollingInterval time.Duration) {
	defaultTimeout := time.Second * 20
	defaultPollingTimeout := time.Second * 5
	switch len(timeout) {
	case 0:
		currentTimeout = defaultTimeout
		pollingInterval = defaultPollingTimeout
	default:
		fallthrough
	case 2:
		pollingInterval = timeout[1]
		if pollingInterval == 0 {
			pollingInterval = defaultPollingTimeout
		}
		fallthrough
	case 1:
		currentTimeout = timeout[0]
		if currentTimeout == 0 {
			// for backwards compatability, leave this zero check
			currentTimeout = defaultTimeout
		}
	}
	return currentTimeout, pollingInterval
}

func (t *testContainer) CurlEventuallyShouldOutput(opts CurlOpts, expectedOutput interface{}, ginkgoOffset int, timeout ...time.Duration) {
	currentTimeout, pollingInterval := getTimeouts(timeout...)

	// for some useful-ish output
	tick := time.Tick(currentTimeout / 8)

	EventuallyWithOffset(ginkgoOffset+1, func(g Gomega) {
		g.Expect(t.CanCurl()).To(BeTrue())

		var res string

		bufChan, done, err := t.CurlAsyncChan(opts)
		if err != nil {
			// trigger an early exit if the pod has been deleted
			// if we return an error here, the Eventually will continue. By making an
			// assertion with the outer context's Gomega, we can trigger a failure at
			// that outer scope.
			g.Expect(err).NotTo(MatchError(ContainSubstring(`pods "testserver" not found`)))
			return
		}
		defer close(done)
		var buf io.Reader
		select {
		case <-tick:
			buf = bytes.NewBufferString("waiting for reply")
		case r, ok := <-bufChan:
			if ok {
				buf = r
			}
		}
		byt, err := io.ReadAll(buf)
		if err != nil {
			res = err.Error()
		} else {
			res = string(byt)
		}

		expectedResponseMatcher := getExpectedResponseMatcher(expectedOutput)
		g.Expect(res).To(expectedResponseMatcher)
		if opts.LogResponses {
			log.GreyPrintf("success: %v", res)
		}

	}, currentTimeout, pollingInterval).Should(Succeed())
}

func (t *testContainer) CurlEventuallyShouldRespond(opts CurlOpts, expectedResponse interface{}, ginkgoOffset int, timeout ...time.Duration) {
	currentTimeout, pollingInterval := getTimeouts(timeout...)
	// for some useful-ish output
	tick := time.Tick(currentTimeout / 8)

	EventuallyWithOffset(ginkgoOffset+1, func(g Gomega) {
		g.Expect(t.CanCurl()).To(BeTrue())

		res, err := t.Curl(opts)
		if err != nil {
			// trigger an early exit if the pod has been deleted.
			// if we return an error here, the Eventually will continue. By making an
			// assertion with the outer context's Gomega, we can trigger a failure at
			// that outer scope.
			g.Expect(err).NotTo(MatchError(ContainSubstring(`pods "testserver" not found`)))
			return
		}
		select {
		default:
			break
		case <-tick:
			if opts.LogResponses {
				log.GreyPrintf("running: %v\nwant %v\nhave: %s", opts, expectedResponse, res)
			}
		}

		expectedResponseMatcher := getExpectedResponseMatcher(expectedResponse)
		g.Expect(res).To(expectedResponseMatcher)
		if opts.LogResponses {
			log.GreyPrintf("success: %v", res)
		}

	}, currentTimeout, pollingInterval).Should(Succeed())
}

// getExpectedResponseMatcher takes an interface and converts it into the types.GomegaMatcher
// that will be used to assert that a given Curl response, matches an expected shape
func getExpectedResponseMatcher(expectedOutput interface{}) types.GomegaMatcher {
	switch a := expectedOutput.(type) {
	case string:
		// In the past, this Curl utility accepted a string, and only asserted that the http response body
		// contained that as a substring.
		// To ensure that all tests which relied on this functionality still work, we accept a string, but
		// improve the assertion to also validate that the StatusCode was a 200
		return WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(&matchers.HttpResponse{
			Body:       ContainSubstring(a),
			StatusCode: http.StatusOK,
		}))
	case *matchers.HttpResponse:
		// There are some cases in tests where we require asserting that a response was not a 200
		// To support that case, we allow developers to supply an HttpResponse object, which defines the
		// expected response. See matchers.HaveHttpResponse for more details
		// This is actually the preferred input, as it means that the WithCurlHttpResponse transform
		// can process the Curl response
		return WithTransform(transforms.WithCurlHttpResponse, matchers.HaveHttpResponse(a))
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

func (t *testContainer) buildCurlArgs(opts CurlOpts) []string {
	curlRequestBuilder := testutils.DefaultCurlRequestBuilder()

	// The testContainer relies on the transforms.WithCurlHttpResponse to validate the response is what
	// we would expect
	// For this transform to behave appropriately, we must execute the request with verbose=true
	curlRequestBuilder.VerboseOutput()

	if opts.WithoutStats {
		curlRequestBuilder.WithoutStats()
	}
	if opts.ReturnHeaders {
		curlRequestBuilder.WithReturnHeaders()
	}

	curlRequestBuilder.WithConnectionTimeout(opts.ConnectionTimeout)

	curlRequestBuilder.WithPath(opts.Path)

	if opts.Method != "" {
		curlRequestBuilder.WithMethod(opts.Method)
	}
	if opts.CaFile != "" {
		curlRequestBuilder.WithCaFile(opts.CaFile)
	}
	if opts.Host != "" {
		curlRequestBuilder.WithHost(opts.Host)
	}
	if opts.Body != "" {
		curlRequestBuilder.WithPostBody(opts.Body)
	}
	for h, v := range opts.Headers {
		curlRequestBuilder.WithHeader(h, v)
	}
	if opts.AllowInsecure {
		curlRequestBuilder.AllowInsecure()
	}

	port := opts.Port
	if port == 0 {
		port = 8080
	}
	curlRequestBuilder.WithPort(port)

	if opts.Protocol != "" {
		curlRequestBuilder.WithScheme(opts.Protocol)
	}

	service := opts.Service
	if service == "" {
		service = "test-ingress"
	}
	curlRequestBuilder.WithService(service)

	if opts.SelfSigned {
		curlRequestBuilder.SelfSigned()
	}
	if opts.Sni != "" {
		curlRequestBuilder.WithSni(opts.Sni)
	}

	args := curlRequestBuilder.BuildArgs()
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
