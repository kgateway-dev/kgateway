package helpers

import "github.com/solo-io/gloo/pkg/api/types/v1"

type MockResolver struct {
	Result string
}

func (m *MockResolver) Resolve(us *v1.Upstream) (string, error) {
	return m.Result, nil
}
