package leaderelector

import (
	"go.uber.org/atomic"
)

var _ Identity = new(identityImpl)

// Identity contains leader election information about the current component
type Identity interface {
	// IsLeader returns true if the current component is the leader, false otherwise
	IsLeader() bool

	// ElectedChannel returns the channel that will be signaled when the current component is elected the leader
	ElectedChannel() <-chan struct{}
}

type identityImpl struct {
	leaderValue    *atomic.Bool
	electedChannel <-chan struct{}
}

func NewIdentity(leaderValue *atomic.Bool, electedChannel <-chan struct{}) *identityImpl {
	return &identityImpl{
		leaderValue:    leaderValue,
		electedChannel: electedChannel,
	}
}

func (i identityImpl) IsLeader() bool {
	return i.leaderValue.Load()
}

func (i identityImpl) ElectedChannel() <-chan struct{} {
	return i.electedChannel
}
