//go:build ignore

package parallel

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

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

// RetryWithLoggedError retries if there is an error and prints that error to stdout
func RetryWithLoggedError() retry.Option {
	return retry.RetryIf(func(err error) bool {
		if err != nil {
			fmt.Println("Retrying because :", err)
		}
		return err != nil
	})
}

// AdvancePortSafe advances the provided port by 1 until it returns a port that is safe to use
// The availability of the port is determined by the errIfPortInUse function
func AdvancePortSafe(p *uint32, errIfPortInUse func(proposedPort uint32) error, retryOptions ...retry.Option) (uint32, error) {
	var newPort uint32

	defaultRetryOptions := []retry.Option{
		RetryWithLoggedError(),

		// We retry here if we are searching for a free port in a test, and the port we attempted was in use
		// We retry a couple of times, and with a delay, to increase the likelihood that we
		// can successfully find a free port
		retry.Attempts(5),
		retry.Delay(time.Millisecond * 100),
	}

	retryErr := retry.Do(func() error {
		newPort = AdvancePort(p)
		return errIfPortInUse(newPort)
	}, append(defaultRetryOptions, retryOptions...)...)

	return newPort, retryErr
}

// MustAdvancePortSafe executes AdvancePortSafe and panics if an error is returned
func MustAdvancePortSafe(p *uint32, errIfPortInUse func(proposedPort uint32) error, retryOptions ...retry.Option) uint32 {
	newPort, err := AdvancePortSafe(p, errIfPortInUse, retryOptions...)
	if err != nil {
		panic(err)
	}
	return newPort
}

// AdvancePort advances the provided port by 1, and adds an offset to support running tests in parallel
func AdvancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(GetPortOffset())
}

// AdvancePortSafeListen returns a port that is safe to use in parallel tests
// It relies on pinging the port to see if it is in use
func AdvancePortSafeListen(p *uint32, retryOptions ...retry.Option) uint32 {
	errIfPortInUse := func(proposedPort uint32) error {
		if err := portInDenyList(proposedPort); err != nil {
			return err
		}
		if err := portInUseListen(proposedPort); err != nil {
			return err
		}
		return nil
	}

	return MustAdvancePortSafe(p, errIfPortInUse, retryOptions...)
}

func portInUseListen(proposedPort uint32) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", proposedPort))

	if err != nil {
		return fmt.Errorf("port %d is in use: %w", proposedPort, err)
	}

	// Port should available if the listener closes without an error
	return ln.Close()
}

var denyListPorts = map[uint32]struct{}{
	// See internal/kgateway/admin/server.go
	// This port is reserved for the admin server
	9097: {},
}

func portInDenyList(proposedPort uint32) error {
	if _, ok := denyListPorts[proposedPort]; ok {
		return fmt.Errorf("port %d is in deny list", proposedPort)
	}
	return nil
}
