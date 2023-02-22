package parallel

import "github.com/onsi/ginkgo/v2"

func init() {
	parallelGinkgoProcesses = ginkgo.GinkgoParallelProcess()
}

var (
	parallelGinkgoProcesses int
)

// GetParallelProcessCount returns the parallel process number for the current ginkgo process
func GetParallelProcessCount() int {
	return parallelGinkgoProcesses
}

// GetPortOffset returns the number of parallel Ginkgo processes * 1000
// This is intended to be used by tests which need to produce unique ports so that they can be run
// in parallel without port conflict
func GetPortOffset() int {
	return parallelGinkgoProcesses * 1000
}
