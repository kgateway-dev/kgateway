package cliutil

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/solo-io/solo-kit/pkg/errors"
)

// Get the resource identified by the given URI.
// The URI can either be an http(s) address or a relative/absolute file path.
func GetResource(uri string) (io.ReadCloser, error) {
	var file io.ReadCloser
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		resp, err := http.Get(uri)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, errors.Errorf("http GET returned status %d", resp.StatusCode)
		}

		file = resp.Body
	} else {
		path, err := filepath.Abs(uri)
		if err != nil {
			return nil, errors.Wrapf(err, "getting absolute path for %v", uri)
		}

		f, err := os.Open(path)
		if err != nil {
			return nil, errors.Wrapf(err, "opening file %v", path)
		}
		file = f
	}

	// Write the body to file
	return file, nil
}
