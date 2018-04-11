package kube

import (
	"strings"

	"encoding/base64"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var InvalidConfigMapErr = errors.New("config map must have exactly 1 value in either BinaryData or Data")

func FileToConfigMap(file *dependencies.File) (*v1.ConfigMap, error) {
	configMapName, key, err := ParseFileRef(file.Ref)
	if err != nil {
		return nil, err
	}
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
	}
	data := string(file.Contents)
	// TODO: make sure configmap.Data can store binary files
	if !utf8.Valid(file.Contents) {
		data = base64.StdEncoding.EncodeToString(file.Contents)
	}
	cm.Data = map[string]string{key: data}
	return cm, nil
}

func ConfigMapToFile(cm *v1.ConfigMap) (*dependencies.File, error) {
	var key string
	var val []byte
	switch {
	case len(cm.BinaryData) == 1:
		if len(cm.Data) > 0 {
			return nil, InvalidConfigMapErr
		}
		for k, v := range cm.BinaryData {
			key = k
			val = v
		}
	case len(cm.Data) == 1:
		if len(cm.BinaryData) > 0 {
			return nil, InvalidConfigMapErr
		}
		for k, v := range cm.Data {
			key = k
			// if the saved content was binary, it was base64'ed
			// get it back
			// note: no longer necessary in kube1.10 whih supports binary data
			if b, err := base64.StdEncoding.DecodeString(v); err == nil {
				v = string(b)
			}
			val = []byte(v)
		}
	}
	if key == "" || len(val) == 0 {
		return nil, InvalidConfigMapErr
	}
	return &dependencies.File{
		Ref:             CreateFileRef(cm.Name, key),
		Contents:        val,
		ResourceVersion: cm.ResourceVersion,
	}, nil
}

func CreateFileRef(configMapName, key string) string {
	return configMapName + ":" + key
}

func ParseFileRef(fileRef string) (string, string, error) {
	parts := strings.Split(fileRef, ":")
	if len(parts) != 2 {
		return "", "", errors.Errorf("invalid file ref for kubernetes: %v. file refs for "+
			"kubernetes must follow the format <configmap_name>:<key_name>", fileRef)
	}
	return parts[0], parts[1], nil
}
