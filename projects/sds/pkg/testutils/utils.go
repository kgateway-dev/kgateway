package testutils

import "io/ioutil"

// FilesToBytes reads the given n files and returns
// an array of the contents
func FilesToBytes(files ...string) [][]byte {
	fileContents := [][]byte{}
	for _, file := range files {
		fileBytes, err := ioutil.ReadFile(file)
		Expect(err).NotTo(HaveOccurred())
		fileContents = append(fileContents, fileBytes)
	}
	return fileContents
}
