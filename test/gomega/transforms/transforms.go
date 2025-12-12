package transforms

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/exp/maps"

	. "github.com/onsi/gomega"
)

const (
	invalidDecompressorResponse = "Failed to decompress bytes"
)

// WithDecompressorTransform returns a Gomega Transform that decompresses
// a slice of bytes and returns the corresponding string
func WithDecompressorTransform() func(b []byte) string {
	return func(b []byte) string {
		reader, err := gzip.NewReader(bytes.NewBuffer(b))
		if err != nil {
			return invalidDecompressorResponse
		}
		defer reader.Close()
		body, err := io.ReadAll(reader)
		if err != nil {
			return invalidDecompressorResponse
		}

		return string(body)
	}
}

// WithHeaderValues returns a Gomega Transform that extracts the header
// values from the http Response, for the provided header name
func WithHeaderValues(header string) func(response *http.Response) []string {
	return func(response *http.Response) []string {
		return response.Header.Values(header)
	}
}

// WithJsonBody returns a Gomega Transform that extracts the JSON body from the
// response and returns it as a map[string]interface{}
func WithJsonBody() func(b []byte) map[string]any {
	return func(b []byte) map[string]any {
		// parse the response body as JSON
		var bodyJson map[string]any
		json.Unmarshal(b, &bodyJson)

		return bodyJson
	}
}

// WithHeaderKeys returns a Gomega Transform that extracts the header keys in a request
func WithHeaderKeys() func(response *http.Response) []string {
	return func(response *http.Response) []string {
		return maps.Keys(response.Header)
	}
}

// BytesToInt converts a byte slice (e.g. a curl response body) to an integer
func BytesToInt(b []byte) int {
	i, err := strconv.Atoi(string(b))
	Expect(err).NotTo(HaveOccurred())
	return i
}
