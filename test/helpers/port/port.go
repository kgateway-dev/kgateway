package port

import (
	"sync/atomic"

	"github.com/onsi/ginkgo/config"
)

var MaxTests = 1000

type TestPort struct {
	port *uint32
}

// Helps you get a free port with ginkgo tests.
func NewTestPort() TestPort {
	return TestPort{
		port: new(uint32),
	}
}

func (t TestPort) NextPort() uint32 {
	return atomic.AddUint32(t.port, 1) + uint32(config.GinkgoConfig.ParallelNode*MaxTests)
}
