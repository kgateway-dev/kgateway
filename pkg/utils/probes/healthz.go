package probes

import "context"

// StartLivenessProbeServer starts a probe server listening on 8765 for requests to /healthz
// and responds with HTTP 200 with body OK\n
//
// This is a convenience wrapper around StartProbeServer which can be customized with params.
func StartLivenessProbeServer(ctx context.Context) {
	StartServer(ctx, NewServerParams())
}
