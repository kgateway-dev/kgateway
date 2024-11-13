package pluginutils

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/golang/protobuf/proto"
	anypb "github.com/golang/protobuf/ptypes/any"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func SetExtensionProtocolOptions(out *envoy_config_cluster_v3.Cluster, filterName string, protoext proto.Message) error {
	protoextAny, err := utils.MessageToAny(protoext)
	if err != nil {
		return errors.Wrapf(err, "converting extension "+filterName+" protocol options to struct")
	}
	if out.GetTypedExtensionProtocolOptions() == nil {
		out.TypedExtensionProtocolOptions = make(map[string]*anypb.Any)
	}

	out.GetTypedExtensionProtocolOptions()[filterName] = protoextAny
	return nil

}
