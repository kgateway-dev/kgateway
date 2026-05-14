package trafficpolicy

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	extensiondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	httpACLModuleName          = "rust_module"
	httpACLFilterName          = "http-acl"
	httpACLFilterNamePrefix    = "dynamic_modules/" + httpACLFilterName
	httpACLDefaultListenerJSON = `{"defaultAction":"allow"}`
)

type httpACLIR struct {
	config *dynamicmodulesv3.DynamicModuleFilterPerRoute
}

var _ PolicySubIR = &httpACLIR{}

func (h *httpACLIR) Equals(other PolicySubIR) bool {
	otherACL, ok := other.(*httpACLIR)
	if !ok {
		return false
	}
	if h == nil && otherACL == nil {
		return true
	}
	if h == nil || otherACL == nil {
		return false
	}
	return proto.Equal(h.config, otherACL.config)
}

func (h *httpACLIR) Validate() error {
	if h == nil || h.config == nil {
		return nil
	}
	return h.config.ValidateAll()
}

// validateACLCIDRs checks that every CIDR entry in the ACL policy has no host bits
// set (e.g. "172.18.0.0/12" is rejected because the network address is 172.16.0.0/12).
// Bare IP addresses without a prefix are accepted as-is.
func validateACLCIDRs(acl *shared.ACLPolicy) error {
	var invalid []string
	for _, rule := range acl.Rules {
		for _, entry := range rule.CIDRs {
			s := string(entry)
			if !strings.Contains(s, "/") {
				// Bare IP — host bits cannot be set by definition.
				continue
			}
			ip, network, err := net.ParseCIDR(s)
			if err != nil {
				invalid = append(invalid, fmt.Sprintf("%q (%v)", s, err))
				continue
			}
			if !ip.Equal(network.IP) {
				invalid = append(invalid, fmt.Sprintf("%q (host bits set; did you mean %q?)", s, network.String()))
			}
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("acl: invalid CIDR(s): %s", strings.Join(invalid, ", "))
	}
	return nil
}

// constructHttpACL constructs the HTTP ACL policy IR from the traffic policy spec.
func constructHttpACL(in *kgateway.TrafficPolicy, out *trafficPolicySpecIr) error {
	if in.Spec.ACL == nil {
		return nil
	}
	if err := validateACLCIDRs(in.Spec.ACL); err != nil {
		return err
	}
	aclJSON, err := json.Marshal(in.Spec.ACL)
	if err != nil {
		return err
	}
	filterCfg, err := utils.MessageToAny(&wrapperspb.StringValue{
		Value: string(aclJSON),
	})
	if err != nil {
		return err
	}
	out.httpACL = &httpACLIR{
		config: &dynamicmodulesv3.DynamicModuleFilterPerRoute{
			DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
				Name: httpACLModuleName,
			},
			PerRouteConfigName: httpACLFilterName,
			FilterConfig:       filterCfg,
		},
	}
	return nil
}

func (p *trafficPolicyPluginGwPass) handleHttpACL(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, httpACL *httpACLIR) {
	if httpACL == nil || httpACL.config == nil {
		return
	}
	typedFilterConfig.AddTypedConfig(httpACLFilterNamePrefix, httpACL.config)
	if p.httpACLInChain == nil {
		p.httpACLInChain = make(map[string]bool)
	}
	p.httpACLInChain[fcn] = true
}
