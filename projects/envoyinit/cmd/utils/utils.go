package utils

import (
	"bytes"
	"os"

	"github.com/solo-io/gloo/projects/envoyinit/pkg/downward"
)

func GetConfig(inputFile string) (string, error) {
	inreader, err := os.Open(inputFile)
	if err != nil {
		return "", err
	}
	defer inreader.Close()

	var buffer bytes.Buffer
	transformer := downward.NewTransformer()
	err = transformer.Transform(inreader, &buffer)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}
