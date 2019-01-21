package install

import (
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
)

func readFile(url string) ([]byte, error) {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Write the body to file
	return ioutil.ReadAll(resp.Body)
}

func ReadManifest(version, urlTemplate string) ([]byte, error) {
	url := fmt.Sprintf(urlTemplate, version)
	bytes, err := readFile(url)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading gloo manifest for version %s at url %s", version, url)
	}
	return bytes, nil
}
