package listers

import (
	"context"
)

//go:generate mockgen -destination mocks/mock_listers.go -package mocks github.com/solo-io/gloo/v2/pkg/listers NamespaceLister

type NamespaceLister interface {
	List(ctx context.Context) ([]string, error)
}
