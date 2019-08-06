package listers

//go:generate mockgen -destination mocks/mock_listers.go -package mocks github.com/solo-io/gloo/pkg/listers NamespaceLister

type NamespaceLister interface {
	List() ([]string, error)
}
