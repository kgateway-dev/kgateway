package parallel

import (
	"sync/atomic"

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
	return AdvancePortByDelta(p, 1)
}

func AdvancePortByDelta(p *uint32, delta uint32) uint32 {
	return atomic.AddUint32(p, delta) + uint32(GetPortOffset())
}
