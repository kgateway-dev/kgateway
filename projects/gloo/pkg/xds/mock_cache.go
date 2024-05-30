package xds

import (
	"context"
	"fmt"

	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

var (
	// Compile-time assertion
	_ envoycache.SnapshotCache = new(MockXdsCache)
)

// MockXdsCache is a custom implementation for the SnapshotCache interface
// It was copied from `project/gloo/pkg/syncer/syncer_test_helper.MockXdsCache`
// It is located here for 2 reasons:
//  1. It is now co-located with the relevant xds code
//  2. It can be imported by other tests, without introducing import cycles
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
