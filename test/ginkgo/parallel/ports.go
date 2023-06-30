package parallel

import (
	"fmt"
	"net"
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

// AdvancePortSafe advances the provided port by 1 until it returns a port that is safe to use
// The availability of the port is determined by the errIfPortInUse function
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

// AdvancePort advances the provided port by 1, and adds an offset to support running tests in parallel
func AdvancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(GetPortOffset())
}

// AdvancePortSafeListen returns a port that is safe to use in parallel tests
// It relies on pinging the port to see if it is in use
func AdvancePortSafeListen(p *uint32) uint32 {
	return AdvancePortSafe(p, portInUseListen)
}

func portInUseListen(proposedPort uint32) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", proposedPort))

	if err != nil {
		return eris.Wrapf(err, "port %d is in use", proposedPort)
	}

	_ = ln.Close()
	// Port is available
	return nil
}
