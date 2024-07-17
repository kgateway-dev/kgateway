package query

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

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

func backendToRefKey(ref gwv1.BackendObjectReference) backendRefKey {
	const delim = "~"
	return backendRefKey(
		string(ptrOrDefault(ref.Group, "")) + delim +
			string(ptrOrDefault(ref.Kind, "")) + delim +
			string(ref.Name) + delim +
			string(ptrOrDefault(ref.Namespace, "")),
	)
}

func ptrOrDefault[T comparable](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}

func (bm BackendMap[T]) Add(backendRef gwv1.BackendObjectReference, value T) {
	key := backendToRefKey(backendRef)
	bm.items[key] = value
}

func (bm BackendMap[T]) AddError(backendRef gwv1.BackendObjectReference, err error) {
	key := backendToRefKey(backendRef)
	bm.errors[key] = err
}

func (bm BackendMap[T]) get(backendRef gwv1.BackendObjectReference, def T) (T, error) {
	key := backendToRefKey(backendRef)
	if err, ok := bm.errors[key]; ok {
		return def, err
	}
	if res, ok := bm.items[key]; ok {
		return res, nil
	}
	return def, ErrUnresolvedReference
}
