package sanitizer

import (
	"context"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var (
	// Compile-time assertion
	_ XdsSanitizer = new(XdsSanitizers)
)

// XdsSanitizer modifies a provided xds snapshot before it is stored in the xds cache,
// with the goal of cleaning up a potentially invalid xds snapshot before being stored and served.
// It is logically invalid for us to return an error here (translation of resources always needs to
// result in a xds snapshot, so we are resilient to pod restarts); instead we should just return the
// xds snapshot unmodified.
type XdsSanitizer interface {
	SanitizeSnapshot(
		ctx context.Context,
		glooSnapshot *v1snap.ApiSnapshot,
		xdsSnapshot envoycache.Snapshot,
		reports reporter.ResourceReports,
	) envoycache.Snapshot
}

type XdsSanitizers []XdsSanitizer

func (s XdsSanitizers) SanitizeSnapshot(
	ctx context.Context,
	glooSnapshot *v1snap.ApiSnapshot,
	xdsSnapshot envoycache.Snapshot,
	reports reporter.ResourceReports,
) envoycache.Snapshot {
	for _, sanitizer := range s {
		xdsSnapshot = sanitizer.SanitizeSnapshot(ctx, glooSnapshot, xdsSnapshot, reports)
	}
	return xdsSnapshot
}
