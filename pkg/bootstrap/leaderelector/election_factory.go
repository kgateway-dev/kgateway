package leaderelector

import "context"

// ElectionConfig is the set of properties that can be used to configure leader elections
type ElectionConfig struct {
	// The name of the component
	Id string
	// The namespace where the component is running
	Namespace string
	// Callback function that is executed when the current component becomes leader
	OnStartedLeading func(c context.Context)
	// Callback function that is executed when the current component stops leading
	OnStoppedLeading func()
	// Callback function that is executed when a new leader is elected
	OnNewLeader func(leaderId string)
}

// An ElectionFactory is an implementation for running a leader election
type ElectionFactory interface {
	// StartElection begins leader election and returns the Identity of the current component
	StartElection(ctx context.Context, config ElectionConfig) (Identity, error)
}
