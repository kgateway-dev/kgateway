package testutils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"io"
	"net/http"
)

// HttpRequestBuilder simplifies the process of generating http requests in tests
type HttpRequestBuilder struct {
	ctx context.Context

	method string

	scheme   string
	hostname string
	port     uint32
	path     string

	body io.Reader

	host    string
	headers map[string]string
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
		body:     nil,
		host:     "",
		headers:  nil,
	}
}

func (h *HttpRequestBuilder) WithContext(ctx context.Context) *HttpRequestBuilder {
	h.ctx = ctx
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

func (h *HttpRequestBuilder) WithPostBodyString(body string) *HttpRequestBuilder {
	return h.WithPostBody(bytes.NewBufferString(body))
}

func (h *HttpRequestBuilder) WithPostBody(body io.Reader) *HttpRequestBuilder {
	h.method = http.MethodPost
	h.body = body
	return h
}

func (h *HttpRequestBuilder) WithHost(host string) *HttpRequestBuilder {
	h.host = host
	return h
}

func (h *HttpRequestBuilder) WithContentType(contentType string) *HttpRequestBuilder {
	h.headers["Content-Type"] = contentType
	return h
}

func (h *HttpRequestBuilder) WithHeader(key, value string) *HttpRequestBuilder {
	h.headers[key] = value
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

func (h *HttpRequestBuilder) Build() *http.Request {
	if err := h.errorIfInvalid(); err != nil {
		// We error loudly here
		// These types of errors are intended to prevent developers from creating resources
		// which are semantically correct, but lead to test flakes/confusion
		ginkgo.Fail(err.Error())
	}

	request, err := http.NewRequestWithContext(
		h.ctx,
		h.method,
		fmt.Sprintf("%s://%s:%d/%s", h.scheme, h.hostname, h.port, h.path),
		h.body)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "generating http request")

	request.Host = h.host
	for headerName, headerValue := range h.headers {
		request.Header.Set(headerName, headerValue)
	}

	return request
}
