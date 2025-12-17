package query

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// BackendMap is a generic type used to manage mappings of backends for Gateway API configurations.
// It's designed to store and manage backend references, like services, and to associate those references
// with either data (of a generic type T) or errors.
type BackendMap[T any] struct {
	items  map[backendRefKey]T
	errors map[backendRefKey]error
}

func NewBackendMap[T any]() BackendMap[T] {
	return BackendMap[T]{
		items:  make(map[backendRefKey]T),
		errors: make(map[backendRefKey]error),
	}
}

type backendRefKey string

func backendToRefKey(ref ir.ObjectSource) backendRefKey {
	const delim = "~"
	return backendRefKey(
		ref.Group + delim +
			ref.Kind + delim +
			ref.Name + delim +
			ref.Namespace,
	)
}

func (bm BackendMap[T]) Add(backendRef ir.ObjectSource, value T) {
	key := backendToRefKey(backendRef)
	bm.items[key] = value
}

func (bm BackendMap[T]) AddError(backendRef ir.ObjectSource, err error) {
	key := backendToRefKey(backendRef)
	bm.errors[key] = err
}

func (bm BackendMap[T]) get(backendRef ir.ObjectSource) (T, error) {
	key := backendToRefKey(backendRef)
	res, backendFound := bm.items[key]
	if err, ok := bm.errors[key]; ok {
		return res, err
	}
	if backendFound {
		return res, nil
	}
	return res, ErrUnresolvedReference
}
