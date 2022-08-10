package singlereplica

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/atomic"
)

var _ leaderelector.ElectionFactory = new(singleReplicaElectionFactory)

type singleReplicaElectionFactory struct {
}

func NewSingleReplicaElectionFactory() *singleReplicaElectionFactory {
	return &singleReplicaElectionFactory{}
}

func (f *singleReplicaElectionFactory) StartElection(ctx context.Context, _ leaderelector.ElectionConfig) (leaderelector.Identity, error) {
	contextutils.LoggerFrom(ctx).Debugf("Starting Single Replica Leader Election")
	return Identity(), nil
}

// Identity returns the Identity used in single replica elections
// Since there is only 1 replica, the identity is always considered the "leader"
func Identity() leaderelector.Identity {
	return leaderelector.NewIdentity(atomic.NewBool(true))
}
