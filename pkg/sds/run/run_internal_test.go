package run

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/kgateway-dev/kgateway/v2/pkg/sds/server"
	"github.com/kgateway-dev/kgateway/v2/pkg/sds/testutils"
)

func TestRunWatcherLoopSerializesDebouncedUpdates(t *testing.T) {
	watcher, watchedFile := newTestWatcher(t)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slogDiscard()
	firstUpdateStarted := make(chan struct{}, 1)
	secondUpdateStarted := make(chan struct{}, 1)
	releaseFirstUpdate := make(chan struct{})
	var updatesStarted int32
	var activeUpdates int32
	var maxActiveUpdates int32

	done := make(chan struct{})
	go func() {
		runWatcherLoop(ctx, watcher, logger, func(context.Context) {
			active := atomic.AddInt32(&activeUpdates, 1)
			for {
				currentMax := atomic.LoadInt32(&maxActiveUpdates)
				if active <= currentMax || atomic.CompareAndSwapInt32(&maxActiveUpdates, currentMax, active) {
					break
				}
			}

			updateNumber := atomic.AddInt32(&updatesStarted, 1)
			switch updateNumber {
			case 1:
				firstUpdateStarted <- struct{}{}
				<-releaseFirstUpdate
			case 2:
				secondUpdateStarted <- struct{}{}
			}

			atomic.AddInt32(&activeUpdates, -1)
		})
		close(done)
	}()

	triggerWatcherEvent(t, watchedFile, "first")
	waitForSignal(t, firstUpdateStarted, "first debounced update")

	triggerWatcherEvent(t, watchedFile, "second")
	time.Sleep(sdsUpdateDebounce + 200*time.Millisecond)

	if got := atomic.LoadInt32(&updatesStarted); got != 1 {
		t.Fatalf("expected the second update to wait for the first to finish, started=%d", got)
	}

	close(releaseFirstUpdate)
	waitForSignal(t, secondUpdateStarted, "second debounced update")

	cancel()
	waitForSignal(t, done, "watcher loop shutdown")

	if got := atomic.LoadInt32(&updatesStarted); got != 2 {
		t.Fatalf("expected exactly two debounced updates, got %d", got)
	}
	if got := atomic.LoadInt32(&maxActiveUpdates); got != 1 {
		t.Fatalf("expected updates to run serially, max concurrency=%d", got)
	}
}

func TestRunWatcherLoopSkipsPendingUpdateAfterCancel(t *testing.T) {
	watcher, watchedFile := newTestWatcher(t)
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slogDiscard()
	firstUpdateStarted := make(chan struct{}, 1)
	releaseFirstUpdate := make(chan struct{})
	var updatesStarted int32

	done := make(chan struct{})
	go func() {
		runWatcherLoop(ctx, watcher, logger, func(context.Context) {
			updateNumber := atomic.AddInt32(&updatesStarted, 1)
			if updateNumber == 1 {
				firstUpdateStarted <- struct{}{}
				<-releaseFirstUpdate
			}
		})
		close(done)
	}()

	triggerWatcherEvent(t, watchedFile, "first")
	waitForSignal(t, firstUpdateStarted, "first debounced update")

	triggerWatcherEvent(t, watchedFile, "second")
	cancel()
	close(releaseFirstUpdate)

	waitForSignal(t, done, "watcher loop shutdown")

	if got := atomic.LoadInt32(&updatesStarted); got != 1 {
		t.Fatalf("expected pending debounced update to be dropped on cancel, got %d updates", got)
	}
}

func TestRunWatcherLoopRecoversFromSingleObservedTornRotation(t *testing.T) {
	keyA, certA, caA := testutils.MustSelfSignedPEM()
	keyB, certB, _ := testutils.MustSelfSignedPEMRotation1()

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	certPath := filepath.Join(dir, "cert.pem")
	caPath := filepath.Join(dir, "ca.pem")
	for path, contents := range map[string][]byte{
		keyPath:  keyA,
		certPath: certA,
		caPath:   caA,
	} {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}

	srv := server.SetupEnvoySDS([]server.Secret{{
		SslKeyFile:        keyPath,
		SslCertFile:       certPath,
		SslCaFile:         caPath,
		ServerCert:        "server",
		ValidationContext: "vc",
	}}, "client", "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.UpdateSDSConfig(ctx); err != nil {
		t.Fatalf("seeding initial snapshot: %v", err)
	}

	watcher, err := newWatcherForFile(keyPath)
	if err != nil {
		t.Fatalf("creating watcher: %v", err)
	}
	defer watcher.Close()

	logger := slogDiscard()
	updateStarted := make(chan struct{}, 1)
	certUpdated := make(chan struct{})
	updateDone := make(chan error, 1)
	loopDone := make(chan struct{})
	var updatesStarted int32

	go func() {
		runWatcherLoop(ctx, watcher, logger, func(ctx context.Context) {
			if atomic.AddInt32(&updatesStarted, 1) == 1 {
				updateStarted <- struct{}{}
			}
			updateDone <- srv.UpdateSDSConfig(ctx)
		})
		close(loopDone)
	}()

	if err := os.WriteFile(keyPath, keyB, 0o600); err != nil {
		t.Fatalf("writing rotated key: %v", err)
	}

	waitForSignal(t, updateStarted, "debounced update to start")

	time.Sleep(150 * time.Millisecond)
	select {
	case err := <-updateDone:
		t.Fatalf("expected update to still be retrying before cert catches up, got %v", err)
	default:
	}

	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := os.WriteFile(certPath, certB, 0o600); err != nil {
			updateDone <- err
			return
		}
		close(certUpdated)
	}()

	waitForSignal(t, certUpdated, "matching cert write during retry window")

	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("expected single observed event to recover after cert catches up: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for debounced update to finish")
	}

	time.Sleep(sdsUpdateDebounce + 200*time.Millisecond)
	if got := atomic.LoadInt32(&updatesStarted); got != 1 {
		t.Fatalf("expected exactly one debounced update from a single observed event, got %d", got)
	}

	cancel()
	waitForSignal(t, loopDone, "watcher loop shutdown")
}

func newTestWatcher(t *testing.T) (*fsnotify.Watcher, string) {
	t.Helper()

	dir := t.TempDir()
	watchedFile := filepath.Join(dir, "tls.crt")
	if err := os.WriteFile(watchedFile, []byte("initial"), 0o600); err != nil {
		t.Fatalf("writing watched file: %v", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("creating watcher: %v", err)
	}
	if err := watcher.Add(watchedFile); err != nil {
		watcher.Close()
		t.Fatalf("adding file to watcher: %v", err)
	}

	return watcher, watchedFile
}

func newWatcherForFile(watchedFile string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(watchedFile); err != nil {
		watcher.Close()
		return nil, err
	}
	return watcher, nil
}

func triggerWatcherEvent(t *testing.T, watchedFile, contents string) {
	t.Helper()

	if err := os.WriteFile(watchedFile, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing watched file: %v", err)
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func slogDiscard() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
