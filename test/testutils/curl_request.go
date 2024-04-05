package testutils

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/onsi/ginkgo/v2"
)

// CurlRequestBuilder simplifies the process of generating curl requests in tests
type CurlRequestBuilder struct {
	verbose           bool
	allowInsecure     bool
	selfSigned        bool
	withoutStats      bool
	connectionTimeout int // seconds
	returnHeaders     bool
	method            string
	host              string
	port              int
	headers           map[string]string
	body              string
	service           string
	sni               string
	caFile            string
	path              string

	scheme string

	additionalArgs []string
}

// DefaultCurlRequestBuilder returns a CurlRequestBuilder with some default values
func DefaultCurlRequestBuilder() *CurlRequestBuilder {
	return &CurlRequestBuilder{
		verbose:           false,
		allowInsecure:     false,
		selfSigned:        false,
		withoutStats:      false,
		connectionTimeout: 3,
		returnHeaders:     false,
		method:            http.MethodGet,
		host:              "",
		port:              8080,
		headers:           make(map[string]string),
		scheme:            "http", // https://github.com/golang/go/issues/40587
		service:           "",
		sni:               "",
		caFile:            "",
		path:              "",

		additionalArgs: []string{},
	}
}

func (c *CurlRequestBuilder) VerboseOutput() *CurlRequestBuilder {
	c.verbose = true
	return c
}

func (c *CurlRequestBuilder) AllowInsecure() *CurlRequestBuilder {
	c.verbose = true
	return c
}

func (c *CurlRequestBuilder) SelfSigned() *CurlRequestBuilder {
	c.selfSigned = true
	return c
}

func (c *CurlRequestBuilder) WithoutStats() *CurlRequestBuilder {
	c.withoutStats = true
	return c
}

func (c *CurlRequestBuilder) WithReturnHeaders() *CurlRequestBuilder {
	c.returnHeaders = true
	return c
}

func (c *CurlRequestBuilder) WithConnectionTimeout(seconds int) *CurlRequestBuilder {
	c.connectionTimeout = seconds
	return c
}

func (c *CurlRequestBuilder) WithMethod(method string) *CurlRequestBuilder {
	c.method = method
	return c
}

func (c *CurlRequestBuilder) WithPort(port int) *CurlRequestBuilder {
	c.port = port
	return c
}

func (c *CurlRequestBuilder) WithService(service string) *CurlRequestBuilder {
	c.service = service
	return c
}

func (c *CurlRequestBuilder) WithSni(sni string) *CurlRequestBuilder {
	c.sni = sni
	return c
}

func (c *CurlRequestBuilder) WithCaFile(caFile string) *CurlRequestBuilder {
	c.caFile = caFile
	return c
}

func (c *CurlRequestBuilder) WithPath(path string) *CurlRequestBuilder {
	c.path = path
	return c
}

func (c *CurlRequestBuilder) WithPostBody(body string) *CurlRequestBuilder {
	return c.WithBody(body).WithContentType("application/json")
}

func (c *CurlRequestBuilder) WithBody(body string) *CurlRequestBuilder {
	c.body = body
	return c
}

func (c *CurlRequestBuilder) WithContentType(contentType string) *CurlRequestBuilder {
	return c.WithHeader("Content-Type", contentType)
}

func (c *CurlRequestBuilder) WithHost(host string) *CurlRequestBuilder {
	return c.WithHeader("Host", host)
}

func (c *CurlRequestBuilder) WithHeader(key, value string) *CurlRequestBuilder {
	c.headers[key] = value
	return c
}

func (c *CurlRequestBuilder) WithScheme(scheme string) *CurlRequestBuilder {
	c.scheme = scheme
	return c
}

// WithArgs allows developers to append arbitrary args to the CurlRequestBuilder
// This should mainly be used for debugging purposes. If there is an argument that the builder
// doesn't yet support, it should be added explicitly, to make it easier for developers to utilize
func (c *CurlRequestBuilder) WithArgs(args []string) *CurlRequestBuilder {
	c.additionalArgs = args
	return c
}

func (c *CurlRequestBuilder) errorIfInvalid() error {
	if c.service == "" {
		return errors.New("service is empty, but required")
	}

	return nil
}

func (c *CurlRequestBuilder) BuildArgs() []string {
	ginkgo.GinkgoHelper()

	if err := c.errorIfInvalid(); err != nil {
		// We error loudly here
		// These types of errors are intended to prevent developers from creating resources
		// which are semantically correct, but lead to test flakes/confusion
		ginkgo.Fail(err.Error())
	}

	args := []string{"curl"}

	if c.verbose {
		args = append(args, "-v")
	}
	if c.allowInsecure {
		args = append(args, "-k")
	}
	if c.withoutStats {
		args = append(args, "-s")
	}
	if c.connectionTimeout > 0 {
		seconds := fmt.Sprintf("%v", c.connectionTimeout)
		args = append(args, "--connect-timeout", seconds, "--max-time", seconds)
	}
	if c.returnHeaders {
		args = append(args, "-I")
	}
	if c.method != http.MethodGet && c.method != "" {
		args = append(args, "-X"+c.method)
	}
	for h, v := range c.headers {
		args = append(args, "-H", fmt.Sprintf("%v: %v", h, v))
	}
	if c.caFile != "" {
		args = append(args, "--cacert", c.caFile)
	}
	if c.body != "" {
		args = append(args, "-d", c.body)
	}
	if c.selfSigned {
		args = append(args, "-k")
	}
	if len(c.additionalArgs) > 0 {
		args = append(args, c.additionalArgs...)
	}
	if c.sni != "" {
		sniResolution := fmt.Sprintf("%s:%d:%s", c.sni, c.port, c.service)
		fullAddress := fmt.Sprintf("%s://%s:%d", c.scheme, c.sni, c.port)
		args = append(args, "--resolve", sniResolution, fullAddress)
	} else {
		args = append(args, fmt.Sprintf("%v://%s:%v%s", c.scheme, c.service, c.port, c.path))
	}

	return args
}
