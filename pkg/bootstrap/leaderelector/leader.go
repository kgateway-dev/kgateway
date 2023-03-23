package leaderelector

import (
	"context"
	"sync"

	"github.com/solo-io/go-utils/contextutils"
)

type LeaderStartupAction struct {
	identity Identity

	actionLock sync.RWMutex
	action     func() error
}

func NewLeaderStartupAction(identity Identity) *LeaderStartupAction {
	return &LeaderStartupAction{
		identity: identity,
	}
}

func (a *LeaderStartupAction) SetAction(action func() error) {
	a.actionLock.Lock()
	a.action = action
	a.actionLock.Unlock()
}

func (a *LeaderStartupAction) GetAction() func() error {
	a.actionLock.RLock()
	defer a.actionLock.RUnlock()
	return a.action
}

func (a *LeaderStartupAction) WatchElectionResults(ctx context.Context) {

	if a.identity.Elected() == nil {
		// no election channel, return early
		return
	}

	doPerformAction := func() {
		contextutils.LoggerFrom(ctx).Debug("performing leader startup action")

		action := a.GetAction()
		if action == nil {
			// This can happen at the beginning of a process, where the leader is immediately elected
			// and no startup action is required to be performed
			contextutils.LoggerFrom(ctx).Debug("leader startup action not defined")
			return
		}
		err := action()
		if err != nil {
			contextutils.LoggerFrom(ctx).Warnw("failed to perform leader startup action", "error", err)
		}
	}

	go func(electionCtx context.Context) {
		// blocking select on multiple channels
		// if either compeltes we are either done or a leader so dont have to busy loop
		select {
		case <-electionCtx.Done():
			return
		case <-a.identity.Elected():
			// channel is closed, signaling leadership
			doPerformAction()
			return
		}

	}(ctx)
}
