package admincli

import (
	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	anypb "github.com/golang/protobuf/ptypes/any"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

// GetStaticClustersByName returns a map of static clusters, indexed by their name
// If there are no static clusters present, an empty map is returned
// An error is returned if any conversion fails
func GetStaticClustersByName(configDump *adminv3.ConfigDump) (map[string]*clusterv3.Cluster, error) {
	clustersByName := make(map[string]*clusterv3.Cluster, 10)
	for _, c := range configDump.Configs {
		staticCluster, err := convertToStaticCluster(c)
		if err != nil {
			return nil, err
		}
		cluster, err := convertToCluster(staticCluster.Cluster)
		if err != nil {
			return nil, err
		}
		clustersByName[cluster.GetName()] = cluster
	}

	return clustersByName, nil
}

// GetStaticCluster returns the static cluster from a ConfigDump, with the given name
// If the cluster is not present, an error is returned
func GetStaticCluster(configDump *adminv3.ConfigDump, staticClusterName string) (*clusterv3.Cluster, error) {
	clusters, err := GetStaticClustersByName(configDump)
	if err != nil {
		return nil, err
	}
	cluster, ok := clusters[staticClusterName]
	if !ok {
		return nil, errors.Errorf("Could not find static cluster with name: %s", staticClusterName)
	}

	return cluster, nil
}

func convertToStaticCluster(a *anypb.Any) (*adminv3.ClustersConfigDump_StaticCluster, error) {
	msg, err := utils.AnyToMessage(a)
	if err != nil {
		return nil, err
	}
	return msg.(*adminv3.ClustersConfigDump_StaticCluster), nil
}

func convertToCluster(a *anypb.Any) (*clusterv3.Cluster, error) {
	msg, err := utils.AnyToMessage(a)
	if err != nil {
		return nil, err
	}
	return msg.(*clusterv3.Cluster), nil
}
