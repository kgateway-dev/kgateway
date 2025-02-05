package testutils

import "os"

// FilesToBytes reads the given n files and returns
// an array of the contents
func FilesToBytes(files ...string) ([][]byte, error) {
	fileContents := [][]byte{}
	for _, file := range files {
		fileBytes, err := os.ReadFile(file)
		if err != nil {
			return fileContents, err
		}
		fileContents = append(fileContents, fileBytes)
	}
	return fileContents, nil
}
