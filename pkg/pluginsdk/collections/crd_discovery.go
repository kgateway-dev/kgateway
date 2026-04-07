package collections

import (
	"context"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type servedVersions struct {
	Served        map[string]bool
	Exists        bool
	Authoritative bool
}

func getServedVersions(extClient apiextensionsclient.Interface, crdName string, versions ...string) servedVersions {
	result := servedVersions{
		Served: make(map[string]bool, len(versions)),
	}
	if extClient == nil {
		return result
	}

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), crdName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			result.Authoritative = true
			return result
		}
		return result
	}

	result.Exists = true
	result.Authoritative = true
	for _, version := range crd.Spec.Versions {
		if !version.Served {
			continue
		}
		for _, requested := range versions {
			if version.Name == requested {
				result.Served[requested] = true
			}
		}
	}

	return result
}
