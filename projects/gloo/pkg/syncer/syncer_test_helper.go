package syncer

import (
	"context"
	"fmt"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

type MockXdsCache struct {
	Called bool
	// Snap that is set
	SetSnap envoycache.Snapshot
	// Snap that is returned
	GetSnap envoycache.Snapshot
}

func (*MockXdsCache) CreateWatch(envoycache.Request) (value chan envoycache.Response, cancel func()) {
	panic("implement me")
}

func (*MockXdsCache) Fetch(context.Context, envoycache.Request) (*envoycache.Response, error) {
	panic("implement me")
}

func (*MockXdsCache) GetStatusInfo(string) envoycache.StatusInfo {
	panic("implement me")
}

func (c *MockXdsCache) GetStatusKeys() []string {
	return []string{}
}

func (c *MockXdsCache) SetSnapshot(node string, snapshot envoycache.Snapshot) error {
	c.Called = true
	c.SetSnap = snapshot
	return nil
}

func (c *MockXdsCache) GetSnapshot(node string) (envoycache.Snapshot, error) {
	if c.GetSnap != nil {
		return c.GetSnap, nil
	}
	return &envoycache.NilSnapshot{}, fmt.Errorf("no snapshot found for node %s", node)
}

func (*MockXdsCache) ClearSnapshot(node string) {
	panic("implement me")
}

type MockXdsSanitizer struct {
	Called bool
	Snap   envoycache.Snapshot
	Err    error
}

func (s *MockXdsSanitizer) SanitizeSnapshot(ctx context.Context, glooSnapshot *v1.ApiSnapshot, xdsSnapshot envoycache.Snapshot, reports reporter.ResourceReports) (envoycache.Snapshot, error) {
	s.Called = true
	if s.Snap != nil {
		return s.Snap, nil
	}
	if s.Err != nil {
		return nil, s.Err
	}
	return xdsSnapshot, nil
}
