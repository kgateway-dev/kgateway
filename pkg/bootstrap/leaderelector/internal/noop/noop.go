package noop

import (
	"context"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/go-utils/contextutils"
)

var _ leaderelector.ElectionFactory = new(noOpElectionFactory)

type noOpElectionFactory struct {
}

func NewNoOpElectionFactory() *noOpElectionFactory {
	return &noOpElectionFactory{}
}

func (f *noOpElectionFactory) StartElection(ctx context.Context, config leaderelector.ElectionConfig) (leaderelector.Identity, error) {
	// All components are considered leaders since there is assumed to only be a single replica
	contextutils.LoggerFrom(ctx).Debugf("Starting NoOp Leader Election")
	return &leaderelector.IdentityImpl{
		Leader: true,
	}, nil
}
