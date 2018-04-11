package dependencies

import storage "github.com/solo-io/gloo/pkg/storage"

type File struct {
	Ref             string
	Contents        []byte
	ResourceVersion string
}

type Secret struct {
	Name            string
	Data            map[string]string
	ResourceVersion string
}

type FileStorage interface {
	Create(*File) (*File, error)
	Update(*File) (*File, error)
	Delete(name string) error
	Get(name string) (*File, error)
	List() ([]*File, error)
	Watch(handlers ...FileEventHandler) (*storage.Watcher, error)
}

type SecretStorage interface {
	Create(*Secret) (*Secret, error)
	Update(*Secret) (*Secret, error)
	Delete(name string) error
	Get(name string) (*Secret, error)
	List() ([]*Secret, error)
	Watch(handlers ...SecretEventHandler) (*storage.Watcher, error)
}
type FileEventHandler interface {
	OnAdd(updatedList []*File, obj *File)
	OnUpdate(updatedList []*File, newObj *File)
	OnDelete(updatedList []*File, obj *File)
}

type SecretEventHandler interface {
	OnAdd(updatedList []*Secret, obj *Secret)
	OnUpdate(updatedList []*Secret, newObj *Secret)
	OnDelete(updatedList []*Secret, obj *Secret)
}

// FileEventHandlerFuncs is an adaptor to let you easily specify as many or
// as few of the notification functions as you want while still implementing
// FileEventHandler.
type FileEventHandlerFuncs struct {
	AddFunc    func(updatedList []*File, obj *File)
	UpdateFunc func(updatedList []*File, newObj *File)
	DeleteFunc func(updatedList []*File, obj *File)
}

// OnAdd calls AddFunc if it's not nil.
func (r FileEventHandlerFuncs) OnAdd(updatedList []*File, obj *File) {
	if r.AddFunc != nil {
		r.AddFunc(updatedList, obj)
	}
}

// OnUpdate calls UpdateFunc if it's not nil.
func (r FileEventHandlerFuncs) OnUpdate(updatedList []*File, newObj *File) {
	if r.UpdateFunc != nil {
		r.UpdateFunc(updatedList, newObj)
	}
}

// OnDelete calls DeleteFunc if it's not nil.
func (r FileEventHandlerFuncs) OnDelete(updatedList []*File, obj *File) {
	if r.DeleteFunc != nil {
		r.DeleteFunc(updatedList, obj)
	}
}

// SecretEventHandlerFuncs is an adaptor to let you easily specify as many or
// as few of the notification functions as you want while still implementing
// SecretEventHandler.
type SecretEventHandlerFuncs struct {
	AddFunc    func(updatedList []*Secret, obj *Secret)
	UpdateFunc func(updatedList []*Secret, newObj *Secret)
	DeleteFunc func(updatedList []*Secret, obj *Secret)
}

// OnAdd calls AddFunc if it's not nil.
func (r SecretEventHandlerFuncs) OnAdd(updatedList []*Secret, obj *Secret) {
	if r.AddFunc != nil {
		r.AddFunc(updatedList, obj)
	}
}

// OnUpdate calls UpdateFunc if it's not nil.
func (r SecretEventHandlerFuncs) OnUpdate(updatedList []*Secret, newObj *Secret) {
	if r.UpdateFunc != nil {
		r.UpdateFunc(updatedList, newObj)
	}
}

// OnDelete calls DeleteFunc if it's not nil.
func (r SecretEventHandlerFuncs) OnDelete(updatedList []*Secret, obj *Secret) {
	if r.DeleteFunc != nil {
		r.DeleteFunc(updatedList, obj)
	}
}
