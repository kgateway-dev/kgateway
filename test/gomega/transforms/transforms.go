package transforms

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
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

func WithHeaderValues(header string) func(response *http.Response) []string {
	return func(response *http.Response) []string {
		return response.Header.Values(header)
	}
}
