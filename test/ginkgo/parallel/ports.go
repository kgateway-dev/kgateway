package parallel

import (
	"github.com/onsi/ginkgo/v2"
	"sync/atomic"
)

// GetParallelProcessCount returns the parallel process number for the current ginkgo process
func GetParallelProcessCount() int {
	return ginkgo.GinkgoParallelProcess()
}

// GetPortOffset returns the number of parallel Ginkgo processes * 100
// This is intended to be used by tests which need to produce unique ports so that they can be run
// in parallel without port conflict
func GetPortOffset() int {
	return GetParallelProcessCount() * 100
}

func AdvancePort(p *uint32) uint32 {
	return atomic.AddUint32(p, 1) + uint32(GetPortOffset())
}

func AdvancePortByDelta(p *uint32, delta uint32) uint32 {
	return atomic.AddUint32(p, delta) + uint32(GetPortOffset())
}
