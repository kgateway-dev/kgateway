package matchers

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io/ioutil"
)

const (
	invalidDecompressorResponse = "Failed to decompress bytes"
	invalidDecodingResponse     = "Failed to decode bytes"
)

// WithDecompressorTransform returns a Gomega Transform that decompresses
// a slice of bytes and returns the corresponding string
func WithDecompressorTransform() interface{} {
	return func(b []byte) string {
		reader, err := gzip.NewReader(bytes.NewBuffer(b))
		if err != nil {
			return invalidDecompressorResponse
		}
		defer reader.Close()
		body, err := ioutil.ReadAll(reader)
		if err != nil {
			return invalidDecompressorResponse
		}

		return string(body)
	}
}

// WithBase64DecodingTransform returns a Gomega Transform that base64
// decodes a slice of bytes and returns the corresponding string
func WithBase64DecodingTransform() interface{} {
	return func(b []byte) string {
		var dest []byte
		_, err := base64.StdEncoding.Decode(dest, b)
		if err != nil {
			return invalidDecodingResponse
		}
		return string(dest)
	}
}

// WithBase64EncodingTransform returns a Gomega Transform that base64
// encodes a slice of bytes and returns the corresponding string
func WithBase64EncodingTransform() interface{} {
	return func(b []byte) string {
		return base64.StdEncoding.EncodeToString(b)
	}
}
