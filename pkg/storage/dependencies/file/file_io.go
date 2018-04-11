package file

import (
	"io/ioutil"

	"path/filepath"

	"os"

	"strings"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

func writeFile(dir string, file *dependencies.File) error {
	if strings.Contains(file.Ref, "/") {
		return errors.Errorf("file name cannot contain '/': %v", file.Ref)
	}
	return ioutil.WriteFile(filepath.Join(dir, file.Ref), file.Contents, 0644)
}

func readFile(dir, filename string) (*dependencies.File, error) {
	data, err := ioutil.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return nil, errors.Errorf("error reading file: %v", err)
	}
	return &dependencies.File{
		Ref:      filename,
		Contents: data,
	}, nil
}

func writeSecret(dir string, secret *dependencies.Secret) error {
	if strings.Contains(secret.Ref, "/") {
		return errors.Errorf("secret ref cannot contain '/': %v", secret.Ref)
	}
	yml, err := yaml.Marshal(secret.Data)
	if err != nil {
		return errors.Wrap(err, "marshalling secret data to yaml")
	}
	return ioutil.WriteFile(filepath.Join(dir, secret.Ref), yml, 0644)
}

func readSecret(dir, filename string) (*dependencies.Secret, error) {
	yml, err := ioutil.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return nil, errors.Errorf("error reading file: %v", err)
	}
	var data map[string]string
	err = yaml.Unmarshal(yml, &data)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling yaml")
	}
	return &dependencies.Secret{
		Ref:  filename,
		Data: data,
	}, nil
}

func deleteFile(dir, filename string) error {
	return os.Remove(filepath.Join(dir, filename))
}
