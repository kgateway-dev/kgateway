package testutils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// HttpRequestBuilder simplifies the process of generating http requests in tests
type HttpRequestBuilder struct {
	ctx context.Context

	method string

	scheme   string
	hostname string
	port     uint32
	path     string

	body string

	host    string
	headers map[string][]string
}

// DefaultRequestBuilder returns an HttpRequestBuilder with some default values
func DefaultRequestBuilder() *HttpRequestBuilder {
	return &HttpRequestBuilder{
		ctx:      context.Background(),
		method:   http.MethodGet,
		scheme:   "http", // https://github.com/golang/go/issues/40587
		hostname: "localhost",
		port:     0,
		path:     "",
		body:     "",
		host:     "",
		headers:  make(map[string][]string),
	}
}

func (h *HttpRequestBuilder) WithContext(ctx context.Context) *HttpRequestBuilder {
	h.ctx = ctx
	return h
}

func (h *HttpRequestBuilder) WithPostMethod() *HttpRequestBuilder {
	h.method = http.MethodPost
	return h
}

func (h *HttpRequestBuilder) WithOptionsMethod() *HttpRequestBuilder {
	h.method = http.MethodOptions
	return h
}

func (h *HttpRequestBuilder) WithScheme(scheme string) *HttpRequestBuilder {
	h.scheme = scheme
	return h
}

func (h *HttpRequestBuilder) WithHostname(hostname string) *HttpRequestBuilder {
	h.hostname = hostname
	return h
}

func (h *HttpRequestBuilder) WithPort(port uint32) *HttpRequestBuilder {
	h.port = port
	return h
}

func (h *HttpRequestBuilder) WithPath(path string) *HttpRequestBuilder {
	h.path = path
	return h
}

func (h *HttpRequestBuilder) WithBody(body string) *HttpRequestBuilder {
	h.body = body
	return h
}

// WithPostBody is syntactic sugar for updating the Method and Body for a POST request simultaneously
func (h *HttpRequestBuilder) WithPostBody(body string) *HttpRequestBuilder {
	return h.WithBody(body).WithPostMethod()
}

func (h *HttpRequestBuilder) WithHost(host string) *HttpRequestBuilder {
	h.host = host
	return h
}

func (h *HttpRequestBuilder) WithContentType(contentType string) *HttpRequestBuilder {
	return h.WithHeader("Content-Type", contentType)
}

func (h *HttpRequestBuilder) WithAcceptEncoding(acceptEncoding string) *HttpRequestBuilder {
	return h.WithHeader("Accept-Encoding", acceptEncoding)
}

const headerDelimiter = ","

// WithHeader accepts a list of header values, separated by the headerDelimiter
// To set a single value for a header, call:
//
//	WithHeader(`headerName`, `value1`)
//
// To set multiple values for a header, call:
//
//	WithHeader(`headerName`, `value1,value2`)
func (h *HttpRequestBuilder) WithHeader(key, value string) *HttpRequestBuilder {
	h.headers[key] = strings.Split(value, headerDelimiter)
	return h
}

func (h *HttpRequestBuilder) errorIfInvalid() error {
	if h.scheme == "" {
		return errors.New("scheme is empty, but required")
	}
	if h.hostname == "" {
		return errors.New("hostname is empty, but required")
	}
	if h.port == 0 {
		return errors.New("port is empty, but required")
	}
	return nil
}

func (h *HttpRequestBuilder) Clone() *HttpRequestBuilder {
	if h == nil {
		return nil
	}
	clone := new(HttpRequestBuilder)

	clone.ctx = h.ctx
	clone.method = h.method
	clone.scheme = h.scheme
	clone.hostname = h.hostname
	clone.port = h.port
	clone.path = h.path
	clone.body = h.body
	clone.host = h.host
	clone.headers = make(map[string][]string)
	for key, value := range h.headers {
		clone.headers[key] = value
	}
	return clone
}

func (h *HttpRequestBuilder) Build() *http.Request {
	if err := h.errorIfInvalid(); err != nil {
		// We error loudly here
		// These types of errors are intended to prevent developers from creating resources
		// which are semantically correct, but lead to test flakes/confusion
		ginkgo.Fail(err.Error())
	}

	// We instantiate a new buffer each time we build a request
	var requestBody io.Reader
	if h.body != "" {
		requestBody = bytes.NewBufferString(h.body)
	}

	request, err := http.NewRequestWithContext(
		h.ctx,
		h.method,
		fmt.Sprintf("%s://%s:%d/%s", h.scheme, h.hostname, h.port, h.path),
		requestBody)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "generating http request")

	request.Host = h.host
	request.Header = h.headers

	return request
}
