package leaderelector

import (
	"context"
	"sync"
	"time"

	"github.com/solo-io/go-utils/contextutils"
)

type LeaderStartupAction struct {
	identity Identity

	lock          sync.RWMutex
	startupAction func() error
}

func NewLeaderStartupAction(identity Identity) *LeaderStartupAction {
	return &LeaderStartupAction{
		identity: identity,
	}
}

func (a *LeaderStartupAction) SetStartupAction(action func() error) {
	contextutils.LoggerFrom(context.Background()).Error("SET STARTUP ACTION")
	a.lock.Lock()
	defer a.lock.Unlock()
	a.startupAction = action
}

func (a *LeaderStartupAction) GetStartupAction() func() error {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.startupAction
}

func (a *LeaderStartupAction) WatchElectionResults(ctx context.Context) {
	var retryChan <-chan time.Time

	doPerformStartupAction := func() {
		startupAction := a.GetStartupAction()
		if startupAction == nil {
			return
		}
		err := startupAction()
		if err != nil {
			contextutils.LoggerFrom(ctx).Warnw("failed to perform leader startup action; will try again shortly.", "error", err)
			retryChan = time.After(time.Second)
		} else {
			retryChan = nil
		}
	}

	go func(electionCtx context.Context) {
		for {
			select {
			case <-electionCtx.Done():
				return
			case <-retryChan:
				doPerformStartupAction()
			case _, ok := <-a.identity.ElectedChannel():
				if !ok {
					// channel has been closed
					return
				}
				doPerformStartupAction()
			default:
				// receiving from other channels would block
			}
		}
	}(ctx)
}
