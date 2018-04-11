package kube

import (
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func secretToKubeSecret(secret *dependencies.Secret) *v1.Secret {
	data := make(map[string][]byte)
	for k, v := range secret.Data {
		data[k] = []byte(v)
	}
	kubeSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            secret.Ref,
			ResourceVersion: secret.ResourceVersion,
		},
		Data: data,
	}
	return kubeSecret
}

func kubeSecretToSecret(kubeSecret *v1.Secret) *dependencies.Secret {
	data := make(map[string]string)
	for k, v := range kubeSecret.Data {
		data[k] = string(v)
	}
	return &dependencies.Secret{
		Ref:             kubeSecret.Name,
		Data:            data,
		ResourceVersion: kubeSecret.ResourceVersion,
	}
}
