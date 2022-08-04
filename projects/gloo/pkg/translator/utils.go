package translator

import (
	"fmt"
	"net"
	"strings"

	errors "github.com/rotisserie/eris"

	envoyal "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// returns the name of the cluster created for a given upstream
func UpstreamToClusterName(upstream *core.ResourceRef) string {

	// For non-namespaced resources, return only name
	if upstream.GetNamespace() == "" {
		return upstream.GetName()
	}

	// Don't use dots in the name as it messes up prometheus stats
	return fmt.Sprintf("%s_%s", upstream.GetName(), upstream.GetNamespace())
}

// returns the ref of the upstream for a given cluster
func ClusterToUpstreamRef(cluster string) (*core.ResourceRef, error) {

	split := strings.Split(cluster, "_")
	if len(split) > 2 || len(split) < 1 {
		return nil, errors.Errorf("unable to convert cluster %s back to upstream ref", cluster)
	}

	ref := &core.ResourceRef{
		Name: split[0],
	}

	if len(split) == 2 {
		ref.Namespace = split[1]
	}
	return ref, nil
}

func NewFilterWithTypedConfig(name string, config proto.Message) (*envoy_config_listener_v3.Filter, error) {

	s := &envoy_config_listener_v3.Filter{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := utils.MessageToAny(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return &envoy_config_listener_v3.Filter{}, err
		}

		s.ConfigType = &envoy_config_listener_v3.Filter_TypedConfig{
			TypedConfig: marshalledConf,
		}
	}

	return s, nil
}

func NewAccessLogWithConfig(name string, config proto.Message) (envoyal.AccessLog, error) {
	s := envoyal.AccessLog{
		Name: name,
	}

	if config != nil {
		marshalledConf, err := utils.MessageToAny(config)
		if err != nil {
			// this should NEVER HAPPEN!
			return envoyal.AccessLog{}, err
		}

		s.ConfigType = &envoyal.AccessLog_TypedConfig{
			TypedConfig: marshalledConf,
		}
	}

	return s, nil
}

func ParseTypedConfig(c typedConfigObject, config proto.Message) error {
	any := c.GetTypedConfig()
	if any != nil {
		return ptypes.UnmarshalAny(any, config)
	}
	return nil
}

type typedConfigObject interface {
	GetTypedConfig() *any.Any
}

// GetIpv6Address returns the IPv6 Address for a provided bindAddress
// returns an error and the original address, if the bindAddress is not a valid IP address
func GetIpv6Address(bindAddress string) (string, error) {
	isIpv4Address, err := IsIpv4Address(bindAddress)
	if err != nil {
		return bindAddress, err
	}

	if isIpv4Address {
		return fmt.Sprintf("::ffff:%s", bindAddress), nil
	}
	return bindAddress, nil
}

// IsIpv4Address returns true if the provided address has a valid IPv4 address
// This is used to distinguish between IPv4 and IPv6 addresses
func IsIpv4Address(bindAddress string) (bool, error) {
	bindIP := net.ParseIP(bindAddress)
	if bindIP == nil {
		// If bindAddress is not a valid textual representation of an IP address
		return false, errors.Errorf("bindAddress %s is not a valid IP address", bindAddress)

	} else if bindIP.To4() != nil {
		// If bindIP is not an IPv4 address, To4 returns nil.
		// Therefore, this is the case where the bindIP represents an IPv4 address
		return true, nil
	}

	return false, nil
}
