package setup

import (
	"context"
	"sync"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/require"
)

type cacheMutationPause struct {
	ctx     context.Context
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newCacheMutationPause(t *testing.T) *cacheMutationPause {
	t.Helper()
	p := &cacheMutationPause{ctx: t.Context(), entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(p.resume)
	return p
}

func (p *cacheMutationPause) wait() {
	close(p.entered)
	select {
	case <-p.release:
	case <-p.ctx.Done():
	}
}

func (p *cacheMutationPause) resume() { p.once.Do(func() { close(p.release) }) }

func runCacheMutation(t *testing.T, f func() error) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- f()
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(proxyUpdateWithin):
			t.Error("cache mutation did not stop after cancellation")
		}
	})
	return done
}

func awaitCacheMutation(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(proxyUpdateWithin):
		t.Fatal("cache mutation did not finish")
	}
}

// Give the second mutation a chance to run before releasing the first. Without
// serialization it finishes here, leaving types from different publications.
func releaseCacheMutation(t *testing.T, pause *cacheMutationPause, second <-chan error) {
	t.Helper()
	select {
	case err := <-second:
		require.NoError(t, err)
	case <-time.After(100 * time.Millisecond):
	}
	pause.resume()
}

func cacheMutationSnapshot(version string) *envoycache.Snapshot {
	cluster := "backend-" + version
	snapshot := clusterEndpointSnapshot(version, version, []string{cluster}, map[string]string{cluster: "10.0.0." + version})
	listeners := listenerRouteSnapshot(version, version, []string{"front-" + version}, map[string]string{"routes-" + version: cluster})
	snapshot.Resources[cachetypes.Listener] = listeners.Resources[cachetypes.Listener]
	snapshot.Resources[cachetypes.Route] = listeners.Resources[cachetypes.Route]
	snapshot.Resources[cachetypes.Secret] = envoycache.NewResources(version, []cachetypes.Resource{&envoytlsv3.Secret{Name: "secret-" + version}})
	return snapshot
}

// Check what fresh watches receive, not only the full snapshot used by admin.
func requireServingSnapshot(t *testing.T, c *perTypeCache, node, version string) {
	t.Helper()
	snapshot, err := c.GetSnapshot(node)
	require.NoError(t, err)
	for _, typeURL := range append([]string{envoyresource.ClusterType}, perTypeOrder...) {
		require.Equal(t, version, snapshot.GetVersion(typeURL))
		var names []string
		want := make(map[string]string)
		for name := range snapshot.GetResources(typeURL) {
			names = append(names, name)
			want[name] = version
		}
		replies := make(chan envoycache.Response, 1)
		cancel, err := c.CreateWatch(&envoycache.Request{
			Node: &envoycorev3.Node{Id: node}, TypeUrl: typeURL, ResourceNames: names,
		}, *newSotwSubscription(names...), replies)
		require.NoError(t, err)
		if cancel != nil {
			t.Cleanup(cancel)
		}
		response := awaitResponse(t, replies, typeURL)
		require.Equal(t, version, response.GetResponseVersion(), typeURL)
		require.Equal(t, want, response.GetReturnedResources(), typeURL)
	}
}

func pauseSnapshotInstall(t *testing.T, c *perTypeCache, node string, pause *cacheMutationPause) <-chan error {
	t.Helper()
	done := runCacheMutation(t, func() error {
		return c.setSnapshot(t.Context(), node, cacheMutationSnapshot("1"), func(typeURL string) {
			if typeURL == envoyresource.ClusterType {
				pause.wait()
			}
		})
	})
	select {
	case <-pause.entered:
	case <-time.After(proxyUpdateWithin):
		t.Fatal("snapshot install did not reach CDS")
	}
	return done
}

func TestPerTypeCacheSerializesSnapshotInstalls(t *testing.T) {
	c := newSnapshotCache(envoycache.IDHash{}, nil).(*perTypeCache)
	pause := newCacheMutationPause(t)
	first := pauseSnapshotInstall(t, c, "node", pause)
	second := runCacheMutation(t, func() error {
		return c.SetSnapshot(t.Context(), "node", cacheMutationSnapshot("2"))
	})
	releaseCacheMutation(t, pause, second)
	awaitCacheMutation(t, first)
	awaitCacheMutation(t, second)
	requireServingSnapshot(t, c, "node", "2")
}

func TestPerTypeCacheClearWaitsForSnapshotInstall(t *testing.T) {
	c := newSnapshotCache(envoycache.IDHash{}, nil).(*perTypeCache)
	pause := newCacheMutationPause(t)
	first := pauseSnapshotInstall(t, c, "node", pause)
	second := runCacheMutation(t, func() error {
		c.ClearSnapshot("node")
		return nil
	})
	releaseCacheMutation(t, pause, second)
	awaitCacheMutation(t, first)
	awaitCacheMutation(t, second)
	for _, cache := range c.caches() {
		_, err := cache.GetSnapshot("node")
		require.Error(t, err, "clear must remove the snapshot from every cache")
	}
	require.Nil(t, c.GetStatusInfo("node"))
}

type pausedClearCache struct {
	envoycache.SnapshotCache
	pause *cacheMutationPause
}

func (c *pausedClearCache) ClearSnapshot(node string) {
	c.SnapshotCache.ClearSnapshot(node)
	c.pause.wait()
}

func TestPerTypeCacheSnapshotInstallWaitsForClear(t *testing.T) {
	c := newSnapshotCache(envoycache.IDHash{}, nil).(*perTypeCache)
	require.NoError(t, c.SetSnapshot(t.Context(), "node", cacheMutationSnapshot("1")))
	pause := newCacheMutationPause(t)
	c.byType[envoyresource.SecretType] = &pausedClearCache{SnapshotCache: c.byType[envoyresource.SecretType], pause: pause}
	first := runCacheMutation(t, func() error {
		c.ClearSnapshot("node")
		return nil
	})
	select {
	case <-pause.entered:
	case <-time.After(proxyUpdateWithin):
		t.Fatal("clear did not reach the secret cache")
	}
	second := runCacheMutation(t, func() error {
		return c.SetSnapshot(t.Context(), "node", cacheMutationSnapshot("2"))
	})
	releaseCacheMutation(t, pause, second)
	awaitCacheMutation(t, first)
	awaitCacheMutation(t, second)
	requireServingSnapshot(t, c, "node", "2")
}

func TestPerTypeCacheDifferentNodesCanPublishIndependently(t *testing.T) {
	c := newSnapshotCache(envoycache.IDHash{}, nil).(*perTypeCache)
	pause := newCacheMutationPause(t)
	first := pauseSnapshotInstall(t, c, "node", pause)
	second := runCacheMutation(t, func() error {
		return c.SetSnapshot(t.Context(), "other", cacheMutationSnapshot("2"))
	})
	awaitCacheMutation(t, second)
	requireServingSnapshot(t, c, "other", "2")
	pause.resume()
	awaitCacheMutation(t, first)
	requireServingSnapshot(t, c, "node", "1")
}
