package localhelpers

import (
	"net/http"
	"io"
	"path/filepath"
	"os"
	"github.com/pkg/errors"
)

func downloadPetstore(destDir string) (string, error) {
	// get us some petstores
	petstoreUrl := "https://s3.amazonaws.com/new-cool-gloo-bucket/petstore"

	resp, err := http.Get(petstoreUrl)
	if err != nil {
		return "", err
	}
	if resp.Body == nil {
		return "", errors.New("no body")
	}
	defer resp.Body.Close()

	filename := filepath.Join(destDir, "petstore")
	petstoreOut, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(petstoreOut, resp.Body); err != nil {
		return "", err
	}

	if err := os.Chmod(filename, 0777); err != nil {
		return "", err
	}

	return filename, nil
}