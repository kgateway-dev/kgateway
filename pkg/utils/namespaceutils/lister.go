package namespaceutils

//go:generate mockgen -destination mocks/mock_lister.go -package mocks github.com/solo-io/gloo/pkg/utils/namespaceutils NamespaceLister

type NamespaceLister interface {
	List() ([]string, error)
}
