package listers

import (
	"context"
)

//go:generate go run github.com/golang/mock/mockgen -destination mocks/mock_listers.go -package mocks github.com/kgateway-dev/kgateway/pkg/listers NamespaceLister

type NamespaceLister interface {
	List(ctx context.Context) ([]string, error)
}
