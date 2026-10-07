package serviceentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/api/label"
	networking "istio.io/api/networking/v1alpha3"
)

// TestSelectedWorkloadFromEntry_Network verifies a WorkloadEntry's network is
// resolved like Istio does: spec.network, else the network label, else the
// system network that pods without a network label also resolve to.
func TestSelectedWorkloadFromEntry_Network(t *testing.T) {
	tests := []struct {
		name           string
		metadataLabels map[string]string
		specNetwork    string
		defaultNetwork string
		want           string
	}{
		{
			name:           "spec network wins",
			metadataLabels: map[string]string{label.TopologyNetwork.Name: "from-label"},
			specNetwork:    "cluster2",
			defaultNetwork: "cluster1",
			want:           "cluster2",
		},
		{
			name:           "network label used without spec network",
			metadataLabels: map[string]string{label.TopologyNetwork.Name: "cluster2"},
			defaultNetwork: "cluster1",
			want:           "cluster2",
		},
		{
			name:           "system network used without spec network or label",
			defaultNetwork: "cluster1",
			want:           "cluster1",
		},
		{
			name: "no network anywhere",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := selectedWorkloadFromEntry(
				"we", "ns",
				tt.metadataLabels,
				nil,
				nil,
				&networking.WorkloadEntry{Address: "1.2.3.4", Network: tt.specNetwork},
				nil,
				tt.defaultNetwork,
			)
			assert.Equal(t, tt.want, workload.network)
			nw, ok := workload.AugmentedLabels[label.TopologyNetwork.Name]
			assert.Equal(t, tt.want != "", ok, "network label presence")
			assert.Equal(t, tt.want, nw)
		})
	}
}
