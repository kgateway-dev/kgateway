package endpointpicker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClusterNameHelpers(t *testing.T) {
	ext := clusterNameExtProc("pool", "ns1")
	assert.Contains(t, ext, "endpointpicker_pool_ns1_ext_proc")
}
