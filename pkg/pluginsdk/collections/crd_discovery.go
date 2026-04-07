package collections

import (
	"context"
	"time"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const crdLookupTimeout = 5 * time.Second

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

	ctx, cancel := context.WithTimeout(context.Background(), crdLookupTimeout)
	defer cancel()

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			result.Authoritative = true
			return result
		}
		return result
	}

	result.Exists = true
	result.Authoritative = true
	requestedVersions := make(map[string]bool, len(versions))
	for _, requested := range versions {
		requestedVersions[requested] = true
	}
	for _, version := range crd.Spec.Versions {
		if !version.Served {
			continue
		}
		if requestedVersions[version.Name] {
			result.Served[version.Name] = true
		}
	}

	return result
}
