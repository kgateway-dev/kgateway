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
	defer a.actionLock.Unlock()
	a.action = action
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
		action := a.GetAction()
		if action == nil {
			return
		}
		err := action()
		if err != nil {
			contextutils.LoggerFrom(ctx).Warnw("failed to perform leader startup action", "error", err)
		}
	}

	go func(electionCtx context.Context) {
		for {
			select {
			case <-electionCtx.Done():
				return
			case <-a.identity.Elected():
				// channel is closed, signaling leadership
				doPerformAction()
				return

			default:
				// receiving from other channels would block
			}
		}
	}(ctx)
}
