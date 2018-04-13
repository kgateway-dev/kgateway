package kube

import (
	"time"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"k8s.io/api/core/v1"
	kubeerrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type secretStorage struct {
	kube kubernetes.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func NewSecretStorage(cfg *rest.Config, namespace string, syncFrequency time.Duration) (dependencies.SecretStorage, error) {
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "creating kube clientset")
	}
	return &secretStorage{
		kube:          kube,
		namespace:     namespace,
		syncFrequency: syncFrequency,
	}, nil
}

func (s *secretStorage) Create(secret *dependencies.Secret) (*dependencies.Secret, error) {
	kubeSecret, err := s.kube.CoreV1().Secrets(s.namespace).Create(secretToKubeSecret(secret))
	if err != nil {
		if kubeerrs.IsAlreadyExists(err) {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("secret %v", secret.Ref))
		}
		return nil, errors.Wrap(err, "kube api call")
	}
	return kubeSecretToSecret(kubeSecret), nil
}

func (s *secretStorage) Update(secret *dependencies.Secret) (*dependencies.Secret, error) {
	kubeSecret, err := s.kube.CoreV1().Secrets(s.namespace).Update(secretToKubeSecret(secret))
	if err != nil {
		return nil, errors.Wrap(err, "kube api call")
	}
	return kubeSecretToSecret(kubeSecret), nil
}

func (s *secretStorage) Delete(name string) error {
	if err := s.kube.CoreV1().Secrets(s.namespace).Delete(name, nil); err != nil {
		return errors.Wrap(err, "kube api call")
	}
	return nil
}

func (s *secretStorage) Get(name string) (*dependencies.Secret, error) {
	secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "kube api call")
	}
	return kubeSecretToSecret(secret), nil

}

func (s *secretStorage) List() ([]*dependencies.Secret, error) {
	var secrets []*dependencies.Secret
	kubeSecretList, err := s.kube.CoreV1().Secrets(s.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "kube api call")
	}
	for _, kubeSecret := range kubeSecretList.Items {
		secrets = append(secrets, kubeSecretToSecret(&kubeSecret))
	}
	return secrets, nil
}

func (s *secretStorage) Watch(handlers ...dependencies.SecretEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(s.kube.CoreV1().RESTClient(),
		"secrets", s.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(v1.Secret), s.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&secretEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

// implements the kubernetes ResourceEventHandler interface
type secretEventHandler struct {
	handler dependencies.SecretEventHandler
	store   cache.Store
}

func secretSecret(obj interface{}) (*dependencies.Secret, bool) {
	kubeSecret, ok := obj.(*v1.Secret)
	if !ok {
		return nil, ok
	}
	return kubeSecretToSecret(kubeSecret), ok
}

func (eh *secretEventHandler) getUpdatedList() []*dependencies.Secret {
	updatedList := eh.store.List()
	var updatedSecretList []*dependencies.Secret
	for _, updated := range updatedList {
		kubeSecret, ok := updated.(*v1.Secret)
		if !ok {
			continue
		}
		updatedSecretList = append(updatedSecretList, kubeSecretToSecret(kubeSecret))
	}
	return updatedSecretList
}

func (eh *secretEventHandler) OnAdd(obj interface{}) {
	secret, ok := secretSecret(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), secret)
}
func (eh *secretEventHandler) OnUpdate(_, newObj interface{}) {
	secret, ok := secretSecret(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), secret)
}

func (eh *secretEventHandler) OnDelete(obj interface{}) {
	secret, ok := secretSecret(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), secret)
}
