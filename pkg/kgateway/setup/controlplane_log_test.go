package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/require"
)

func TestBoundLogArgs(t *testing.T) {
	short := []string{"a", "b"}
	shortErr := errors.New("short")
	require.Equal(t, []any{"type", short, shortErr}, boundLogArgs([]any{"type", short, shortErr}),
		"args within the limits must pass through unchanged")

	names := make([]string, 100)
	for i := range names {
		names[i] = fmt.Sprintf("route-%d", i)
	}
	got := fmt.Sprintf("%v", boundLogArgs([]any{names})...)
	require.Equal(t, "[route-0 route-1 route-2 route-3 route-4 route-5 route-6 route-7 route-8 route-9] ... (100 names total)", got)

	longErr := errors.New(strings.Repeat("x", maxLoggedErrLen+10))
	got = fmt.Sprintf("%v", boundLogArgs([]any{longErr})...)
	require.Equal(t, strings.Repeat("x", maxLoggedErrLen)+" ... (10 bytes truncated)", got)
}

// The go-control-plane ADS cache warns with every requested name when it holds
// back a named response; through the adapter the line must stay bounded no
// matter how many names the proxy requests.
func TestADSNotRespondingWarningIsBounded(t *testing.T) {
	var buf bytes.Buffer
	adapter := &slogAdapterForEnvoy{logger: slog.New(slog.NewTextHandler(&buf, nil))}
	cache := envoycache.NewSnapshotCache(true, retentionHash{}, adapter)

	var requested, held []string
	for i := range 5000 {
		requested = append(requested, fmt.Sprintf("kube_ns_requested-service-%d_8080", i))
		held = append(held, fmt.Sprintf("kube_ns_unrequested-service-%d_8080", i))
	}
	snapshot := edsOnlySnapshot(t, "1", append(append([]string{}, requested...), held...)...)
	require.NoError(t, cache.SetSnapshot(context.Background(), retentionNode, snapshot))

	cancel, err := cache.CreateWatch(namedEDSRequest("", requested...), *newSotwSubscription(requested...), make(chan envoycache.Response, 1))
	require.NoError(t, err)
	defer cancel()

	line := buf.String()
	require.Contains(t, line, "ADS mode: not responding to request", "the warning must still be logged")
	require.Contains(t, line, "(5000 names total)", "the warning must report how many names were requested")
	require.Less(t, len(line), 4096, "the warning must not grow with the number of names: %s", line)
}
