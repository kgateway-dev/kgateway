package bootstrap

import (
	"path/filepath"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	kubemeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// sharedCache OR resourceCrd+cfg must be non-nil
func ConfigFactoryForSettings(settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	cache kube.SharedCache,
	resourceCrd crd.Crd,
	cfg **rest.Config) (factory.ResourceClientFactory, error) {
	if settings.ConfigSource == nil {
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	switch source := settings.ConfigSource.(type) {
	// this is at trick to reuse the same cfg across multiple clients
	case *v1.Settings_KubernetesConfigSource:
		if *cfg == nil {
			c, err := kubeutils.GetConfig("", "")
			if err != nil {
				return nil, err
			}
			*cfg = c
		}
		return &factory.KubeResourceClientFactory{
			Crd:         resourceCrd,
			Cfg:         *cfg,
			SharedCache: cache,
		}, nil
	case *v1.Settings_DirectoryConfigSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectoryConfigSource.Directory, resourceCrd.Plural),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}

// sharedCach OR resourceCrd+cfg must be non-nil
func SecretFactoryForSettings(settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	pluralName string,
	cfg **rest.Config,
	clientset *kubernetes.Interface) (factory.ResourceClientFactory, error) {
	if settings.SecretSource == nil {
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	switch source := settings.SecretSource.(type) {
	case *v1.Settings_KubernetesSecretSource:
		if cfg == nil {
			c, err := kubeutils.GetConfig("", "")
			if err != nil {
				return nil, err
			}
			*cfg = c
		}

		if *clientset == nil {
			cs, err := kubernetes.NewForConfig(*cfg)
			if err != nil {
				return nil, err
			}
			*clientset = cs
		}
		return &factory.KubeSecretClientFactory{
			Clientset: *clientset,
		}, nil
	case *v1.Settings_VaultSecretSource:
		return nil, errors.Errorf("vault configuration not implemented")
	case *v1.Settings_DirectorySecretSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectorySecretSource.Directory, pluralName),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}

// sharedCach OR resourceCrd+cfg must be non-nil
func ArtifactFactoryForSettings(settings *v1.Settings,
	sharedCache memory.InMemoryResourceCache,
	pluralName string,
	cfg **rest.Config,
	clientset *kubernetes.Interface) (factory.ResourceClientFactory, error) {
	if settings.SecretSource == nil {
		if sharedCache == nil {
			return nil, errors.Errorf("internal error: shared cache cannot be nil")
		}
		return &factory.MemoryResourceClientFactory{
			Cache: sharedCache,
		}, nil
	}

	switch source := settings.ArtifactSource.(type) {
	case *v1.Settings_KubernetesArtifactSource:
		if cfg == nil {
			c, err := kubeutils.GetConfig("", "")
			if err != nil {
				return nil, err
			}
			*cfg = c
		}

		if clientset == nil {
			cs, err := kubernetes.NewForConfig(*cfg)
			if err != nil {
				return nil, err
			}
			*clientset = cs
		}
		return &factory.KubeSecretClientFactory{
			Clientset: *clientset,
		}, nil
	case *v1.Settings_DirectoryArtifactSource:
		return &factory.FileResourceClientFactory{
			RootDir: filepath.Join(source.DirectoryArtifactSource.Directory, pluralName),
		}, nil
	}
	return nil, errors.Errorf("invalid config source type")
}

func ListAllNamespaces(cfg *rest.Config) ([]string, error) {
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "creating kube rest client")
	}
	kubeNamespaces, err := kube.CoreV1().Namespaces().List(kubemeta.ListOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "listing kube namespaces")
	}
	var namespaces []string
	for _, ns := range kubeNamespaces.Items {
		namespaces = append(namespaces, ns.Name)
	}
	return namespaces, nil
}
