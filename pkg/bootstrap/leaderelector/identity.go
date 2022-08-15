package leaderelector

import (
	"go.uber.org/atomic"
)

var _ Identity = new(identityImpl)

// Identity contains leader election information about the current component
type Identity interface {
	// IsLeader returns true if the current component is the leader, false otherwise
	IsLeader() bool
}

type identityImpl struct {
	leaderValue *atomic.Bool
}

func NewIdentity(leaderValue *atomic.Bool) *identityImpl {
	return &identityImpl{
		leaderValue: leaderValue,
	}
}

func (i identityImpl) IsLeader() bool {
	return i.leaderValue.Load()
}
