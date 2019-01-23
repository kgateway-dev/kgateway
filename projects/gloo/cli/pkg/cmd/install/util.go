package install

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

func readFile(url string) ([]byte, error) {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("http GET returned status %d", resp.StatusCode)
	}

	// Write the body to file
	return ioutil.ReadAll(resp.Body)
}

func readManifest(version, urlTemplate string) ([]byte, error) {
	url := fmt.Sprintf(urlTemplate, version)
	bytes, err := readFile(url)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading manifest for gloo version %s at url %s", version, url)
	}
	return bytes, nil
}
