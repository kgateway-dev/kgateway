package matchers

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
)

const (
	invalidDecompressorResponse = "Failed to decompress bytes"
)

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
