package localhelpers

import (
	"github.com/pkg/errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func downloadPetstore(destDir string) (string, error) {
	// get us some petstores
	petstoreUrl := "https://raw.githubusercontent.com/ilackarms/go-swagger/master/examples/2.0/petstore/server/petstore"

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
	defer petstoreOut.Close()

	if _, err := io.Copy(petstoreOut, resp.Body); err != nil {
		return "", err
	}

	if err := os.Chmod(filename, 0777); err != nil {
		return "", err
	}

	return filename, nil
}
