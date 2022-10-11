package syncer

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	// Compile-time assertion
	_ sanitizer.XdsSanitizer = new(MockXdsSanitizer)
	// Compile-time assertion
	_ envoycache.SnapshotCache = new(MockXdsCache)
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
func (c *MockXdsCache) SetSnapshot(node string, snapshot envoycache.Snapshot) {
	c.Called = true
	c.SetSnap = snapshot
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
}

func (s *MockXdsSanitizer) SanitizeSnapshot(ctx context.Context, glooSnapshot *v1snap.ApiSnapshot, xdsSnapshot envoycache.Snapshot, reports reporter.ResourceReports) envoycache.Snapshot {
	s.Called = true
	if s.Snap != nil {
		return s.Snap
	}
	return xdsSnapshot
}
