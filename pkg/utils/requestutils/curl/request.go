package curl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/solo-io/go-utils/contextutils"
)

// BuildArgsOrError accepts a set of curl.Option and generates the list of arguments
// that can be used to execute a curl request
// An error is returned if the arguments for the curl request are known to fail
// This is meant to aid the developer experience, since we explicitly error if some
// required parameters were ignored.
func BuildArgsOrError(_ context.Context, options ...Option) ([]string, error) {
	config := &requestConfig{
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
		retry:             0, // do not retry
		retryDelay:        -1,
		retryMaxTime:      0,

		additionalArgs: []string{},
	}

	for _, opt := range options {
		opt(config)
	}

	return config.generateArgsOrError()
}

// BuildArgs invokes BuildArgsOrError and uses a log.DPanic in case an error was encountered
func BuildArgs(ctx context.Context, options ...Option) []string {
	args, err := BuildArgsOrError(ctx, options...)
	if err != nil {
		// An error is returned here to improve the dev experience,
		// as the CurlRequest that was constructed is known to be invalid
		// Therefore, for developers we error loudly using a DPanic
		contextutils.LoggerFrom(ctx).DPanic(err)
	}

	return args
}

// requestConfig contains the set of options that can be used to configure a curl request
type requestConfig struct {
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

	retry        int
	retryDelay   int
	retryMaxTime int

	additionalArgs []string
}

func (c *requestConfig) errorIfInvalid() error {
	if c.service == "" {
		return errors.New("service is empty, but required")
	}

	return nil
}

func (c *requestConfig) generateArgsOrError() ([]string, error) {
	if err := c.errorIfInvalid(); err != nil {
		return nil, err
	}

	var args []string

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

	if c.retry != 0 {
		args = append(args, "--retry", fmt.Sprintf("%d", c.retry))
	}
	if c.retryDelay != -1 {
		args = append(args, "--retry-delay", fmt.Sprintf("%d", c.retryDelay))
	}
	if c.retryMaxTime != 0 {
		args = append(args, "--retry-max-time", fmt.Sprintf("%d", c.retryMaxTime))
	}

	if c.sni != "" {
		sniResolution := fmt.Sprintf("%s:%d:%s", c.sni, c.port, c.service)
		fullAddress := fmt.Sprintf("%s://%s:%d", c.scheme, c.sni, c.port)
		args = append(args, "--resolve", sniResolution, fullAddress)
	} else {
		args = append(args, fmt.Sprintf("%v://%s:%v/%s", c.scheme, c.service, c.port, c.path))
	}

	return args, nil
}
