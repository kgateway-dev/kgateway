package curl

import (
	"strconv"
	"strings"
)

// Option represents an option for a curl request.
type Option func(config *requestConfig)

func VerboseOutput() Option {
	return func(config *requestConfig) {
		config.verbose = true
	}
}

func AllowInsecure() Option {
	return func(config *requestConfig) {
		config.verbose = true
	}
}

func SelfSigned() Option {
	return func(config *requestConfig) {
		config.selfSigned = true
	}
}

func WithoutStats() Option {
	return func(config *requestConfig) {
		config.withoutStats = true
	}
}

func WithReturnHeaders() Option {
	return func(config *requestConfig) {
		config.returnHeaders = true
	}
}

func WithConnectionTimeout(seconds int) Option {
	return func(config *requestConfig) {
		config.connectionTimeout = seconds
	}
}

func WithMethod(method string) Option {
	return func(config *requestConfig) {
		config.method = method
	}
}

func WithPort(port int) Option {
	return func(config *requestConfig) {
		config.port = port
	}
}

func WithService(service string) Option {
	return func(config *requestConfig) {
		config.service = service
	}
}

func WithAddress(address string) Option {
	return func(config *requestConfig) {
		addressParts := strings.Split(address, ":")
		service := addressParts[0]
		// We intentionally drop the error here
		// If one occurred, it means that the developer-defined address was invalid
		// And when the curl request is built and executed, it will fail
		port, _ := strconv.Atoi(addressParts[1])
		WithService(service)(config)
		WithPort(port)(config)
	}
}

func WithSni(sni string) Option {
	return func(config *requestConfig) {
		config.sni = sni
	}
}

func WithCaFile(caFile string) Option {
	return func(config *requestConfig) {
		config.caFile = caFile
	}
}

func WithPath(path string) Option {
	return func(config *requestConfig) {
		config.path = strings.TrimPrefix(path, "/")
	}
}

func WithRetries(retry, retryDelay, retryMaxTime int) Option {
	return func(config *requestConfig) {
		config.retry = retry
		config.retryDelay = retryDelay
		config.retryMaxTime = retryMaxTime
	}
}

func WithPostBody(body string) Option {
	return func(config *requestConfig) {
		WithBody(body)(config)
		WithContentType("application/json")(config)
	}
}

func WithBody(body string) Option {
	return func(config *requestConfig) {
		config.body = body
	}
}

func WithContentType(contentType string) Option {
	return func(config *requestConfig) {
		WithHeader("Content-Type", contentType)(config)
	}
}

func WithHost(host string) Option {
	return func(config *requestConfig) {
		WithHeader("Host", host)(config)
	}
}

func WithHeader(key, value string) Option {
	return func(config *requestConfig) {
		config.headers[key] = value
	}
}

func WithScheme(scheme string) Option {
	return func(config *requestConfig) {
		config.scheme = scheme
	}
}

// WithArgs allows developers to append arbitrary args to the curl request
// This should mainly be used for debugging purposes. If there is an argument that the builder
// doesn't yet support, it should be added explicitly, to make it easier for developers to utilize
func WithArgs(args []string) Option {
	return func(config *requestConfig) {
		config.additionalArgs = args
	}
}
