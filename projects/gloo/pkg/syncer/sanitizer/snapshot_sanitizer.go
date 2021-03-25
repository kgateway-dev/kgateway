package sanitizer

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// an XdsSanitizer modifies an xds snapshot before it is stored in the xds cache
// the if the sanitizer returns an error, Gloo will not update the xds cache with the snapshot
// else Gloo will assume the snapshot is valid to send to Envoy
type XdsSanitizer interface {
	SanitizeSnapshot(
		ctx context.Context,
		glooSnapshot *v1.ApiSnapshot,
		xdsSnapshot envoycache.Snapshot,
		reports reporter.ResourceReports,
	) (envoycache.Snapshot, error)
}

type XdsSanitizers []XdsSanitizer

func (s XdsSanitizers) SanitizeSnapshot(
	ctx context.Context,
	glooSnapshot *v1.ApiSnapshot,
	xdsSnapshot envoycache.Snapshot,
	reports reporter.ResourceReports,
) (envoycache.Snapshot, error) {
	for _, sanitizer := range s {
		var err error
		xdsSnapshot, err = sanitizer.SanitizeSnapshot(ctx, glooSnapshot, xdsSnapshot, reports)
		if err != nil {
			return nil, err
		}
	}
	// Snapshot is consistent, so check if we have errors not related to the upstreams
	if resourcesErr := reports.Validate(); resourcesErr != nil {
		return nil, resourcesErr
	}

	return xdsSnapshot, nil
}
