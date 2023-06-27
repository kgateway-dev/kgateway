package parallel

import "github.com/onsi/ginkgo/v2"

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
