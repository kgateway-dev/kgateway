package parallel

import (
	"sync/atomic"
	"time"

	"github.com/rotisserie/eris"

	"github.com/avast/retry-go"

	"github.com/onsi/ginkgo/v2"
)

// GetParallelProcessCount returns the parallel process number for the current ginkgo process
func GetParallelProcessCount() int {
	return ginkgo.GinkgoParallelProcess()
}

// GetPortOffset returns the number of parallel Ginkgo processes * 1000
// This is intended to be used by tests which need to produce unique ports so that they can be run
// in parallel without port conflict
func GetPortOffset() int {
	return GetParallelProcessCount() * 1000
}

func AdvancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(GetPortOffset())
}

func AdvancePortSafe(p *uint32, errIfPortInUse func(proposedPort uint32) error) uint32 {
	var newPort uint32

	_ = retry.Do(func() error {
		newPort = AdvancePort(p)
		return errIfPortInUse(newPort)
	},
		retry.RetryIf(func(err error) bool {
			return err != nil
		}),
		retry.Attempts(3),
		retry.Delay(time.Millisecond*0))

	return newPort
}

func portInUseDenylist(proposedPort uint32) error {
	var denyList = map[uint32]struct{}{
		10010: {}, // used by Gloo, when devMode is enabled
	}

	if _, ok := denyList[proposedPort]; ok {
		return eris.Errorf("port %d is in use", proposedPort)
	}
	return nil
}

// AdvancePortSafeDenylist returns a port that is safe to use in parallel tests
// It relies on a hard-coded denylist, of ports that we know are hard-coded in Gloo
// And will cause tests to fail if they attempt to bind on the same port
// If you need a more advanced port selection mechanism, use AdvancePortSafe
func AdvancePortSafeDenylist(p *uint32) uint32 {
	return AdvancePortSafe(p, portInUseDenylist)
}
