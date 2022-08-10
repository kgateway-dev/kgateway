package leaderelector

import (
	"go.uber.org/atomic"
)

var _ Identity = new(identityImpl)

type Identity interface {
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
