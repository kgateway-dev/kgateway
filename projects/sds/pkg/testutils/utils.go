package testutils

import "io/ioutil"

// FilesToBytes reads the given n files and returns
// an array of the contents
func FilesToBytes(files ...string) ([][]byte, error) {
	fileContents := [][]byte{}
	for _, file := range files {
		fileBytes, err := ioutil.ReadFile(file)
		if err != nil {
			return fileContents, err
		}
		fileContents = append(fileContents, fileBytes)
	}
	return fileContents, nil
}
