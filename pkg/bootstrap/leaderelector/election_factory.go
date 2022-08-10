package leaderelector

import "context"

type ElectionConfig struct {
	Id               string
	Namespace        string
	OnStartedLeading func(c context.Context)
	OnStoppedLeading func()
	OnNewLeader      func(leaderId string)
}

type ElectionFactory interface {
	StartElection(ctx context.Context, config ElectionConfig) (Identity, error)
}
