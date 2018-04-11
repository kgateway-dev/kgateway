package kube

import (
	"time"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type configMapStorage struct {
	kube kubernetes.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func NewFileStorage(cfg *rest.Config, namespace string, syncFrequency time.Duration) (dependencies.FileStorage, error) {
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "creating kube clientset")
	}
	return &configMapStorage{
		kube:          kube,
		namespace:     namespace,
		syncFrequency: syncFrequency,
	}, nil
}

func (s *configMapStorage) Create(file *dependencies.File) (*dependencies.File, error) {
	cm, err := FileToConfigMap(file)
	if err != nil {
		return nil, errors.Wrap(err, "converting file to config map")
	}
	cm, err = s.kube.CoreV1().ConfigMaps(s.namespace).Create(cm)
	if err != nil {
		return nil, errors.Wrap(err, "kube api call")
	}
	newFile, err := ConfigMapToFile(cm)
	if err != nil {
		return nil, errors.Wrap(err, "converting created config map back to file")
	}
	return newFile, nil
}

func (s *configMapStorage) Update(file *dependencies.File) (*dependencies.File, error) {
	cm, err := FileToConfigMap(file)
	if err != nil {
		return nil, errors.Wrap(err, "converting file to config map")
	}
	cm, err = s.kube.CoreV1().ConfigMaps(s.namespace).Update(cm)
	if err != nil {
		return nil, errors.Wrap(err, "kube api call")
	}
	newFile, err := ConfigMapToFile(cm)
	if err != nil {
		return nil, errors.Wrap(err, "converting updated config map back to file")
	}
	return newFile, nil
}

func (s *configMapStorage) Delete(name string) error {
	cmName, _, err := ParseFileRef(name)
	if err != nil {
		return errors.Wrap(err, "parsing file ref")
	}
	if err := s.kube.CoreV1().ConfigMaps(s.namespace).Delete(cmName, nil); err != nil {
		return errors.Wrap(err, "kube api call")
	}
	return nil
}

func (s *configMapStorage) Get(name string) (*dependencies.File, error) {
	cmName, _, err := ParseFileRef(name)
	if err != nil {
		return nil, errors.Wrap(err, "parsing file ref")
	}
	cm, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(cmName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "kube api call")
	}
	file, err := ConfigMapToFile(cm)
	if err != nil {
		return nil, errors.Wrap(err, "converting config map to file")
	}
	return file, nil

}

func (s *configMapStorage) List() ([]*dependencies.File, error) {
	var files []*dependencies.File
	cmList, err := s.kube.CoreV1().ConfigMaps(s.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "kube api call")
	}
	for _, cm := range cmList.Items {
		file, err := ConfigMapToFile(&cm)
		if err != nil {
			return nil, errors.Wrap(err, "converting config map to file")
		}
		files = append(files, file)
	}
	return files, nil
}

func (s *configMapStorage) Watch(handlers ...dependencies.FileEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(s.kube.CoreV1().RESTClient(),
		"configmaps", s.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(v1.ConfigMap), s.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&configMapEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

// implements the kubernetes ResourceEventHandler interface
type configMapEventHandler struct {
	handler dependencies.FileEventHandler
	store   cache.Store
}

func convertCm(obj interface{}) (*dependencies.File, bool) {
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		return nil, ok
	}
	file, err := ConfigMapToFile(cm)
	if err != nil {
		return nil, false
	}
	return file, ok
}

func (eh *configMapEventHandler) getUpdatedList() []*dependencies.File {
	updatedList := eh.store.List()
	var updatedFileList []*dependencies.File
	for _, updated := range updatedList {
		cm, ok := updated.(*v1.ConfigMap)
		if !ok {
			continue
		}
		file, err := ConfigMapToFile(cm)
		if err != nil {
			continue
		}
		updatedFileList = append(updatedFileList, file)
	}
	return updatedFileList
}

func (eh *configMapEventHandler) OnAdd(obj interface{}) {
	file, ok := convertCm(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), file)
}
func (eh *configMapEventHandler) OnUpdate(_, newObj interface{}) {
	file, ok := convertCm(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), file)
}

func (eh *configMapEventHandler) OnDelete(obj interface{}) {
	file, ok := convertCm(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), file)
}
