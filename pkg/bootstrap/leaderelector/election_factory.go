package leaderelector

import (
	"context"
	"os"
	"strconv"
)

// Leader Election is a valuable feature of Gloo Edge that is enabled by default
// If you wish to disable it, set this env variable to a truthy value ("1", "t", "T", "true", "TRUE", "True")
const disableElectionEnvVar = "DISABLE_LEADER_ELECTION"

var (
	disableElection = false
)

func init() {
	disableElectionVal := os.Getenv(disableElectionEnvVar)
	boolValue, err := strconv.ParseBool(disableElectionVal)
	// in the case where a non-truthy string was provided, this will return an error
	// in that case, we ignore the value altogether
	if err == nil {
		disableElection = boolValue
	}
}

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
	StartElection(ctx context.Context, config *ElectionConfig) (Identity, error)
}

// IsDisabled returns true if leader election is disabled using an environment variable
func IsDisabled() bool {
	return disableElection
}
