package trafficpolicy

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

type httpUpgradeIR struct {
	configs []*envoyroutev3.RouteAction_UpgradeConfig
}

var _ PolicySubIR = &httpUpgradeIR{}

func (h *httpUpgradeIR) Equals(other PolicySubIR) bool {
	otherHTTPUpgrade, ok := other.(*httpUpgradeIR)
	if !ok {
		return false
	}
	if h == nil || otherHTTPUpgrade == nil {
		return h == nil && otherHTTPUpgrade == nil
	}
	return slices.EqualFunc(h.configs, otherHTTPUpgrade.configs, func(a, b *envoyroutev3.RouteAction_UpgradeConfig) bool {
		return proto.Equal(a, b)
	})
}

func (h *httpUpgradeIR) Validate() error {
	if h == nil {
		return nil
	}
	for _, config := range h.configs {
		if err := config.ValidateAll(); err != nil {
			return err
		}
	}
	return nil
}

func constructHTTPUpgrade(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) error {
	if len(spec.HTTPUpgrade) == 0 {
		return nil
	}
	if spec.Buffer != nil && spec.Buffer.Disable == nil {
		return errors.New("buffer cannot be used together with httpUpgrade")
	}

	configs := make([]*envoyroutev3.RouteAction_UpgradeConfig, 0, len(spec.HTTPUpgrade))
	seen := make(map[string]struct{}, len(spec.HTTPUpgrade))
	for _, upgrade := range spec.HTTPUpgrade {
		canonicalType := strings.ToLower(upgrade.Type)
		if _, found := seen[canonicalType]; found {
			return fmt.Errorf("duplicate HTTP upgrade type %q", upgrade.Type)
		}
		seen[canonicalType] = struct{}{}

		config := &envoyroutev3.RouteAction_UpgradeConfig{UpgradeType: upgrade.Type}
		if upgrade.Connect != nil && upgrade.Connect.Terminate != nil && *upgrade.Connect.Terminate {
			config.ConnectConfig = &envoyroutev3.RouteAction_UpgradeConfig_ConnectConfig{}
		}
		configs = append(configs, config)
	}

	out.httpUpgrade = &httpUpgradeIR{configs: configs}
	return nil
}

func applyHTTPUpgrade(upgrade *httpUpgradeIR, action *envoyroutev3.RouteAction) {
	if upgrade == nil || action == nil {
		return
	}

	for _, desired := range upgrade.configs {
		idx := slices.IndexFunc(action.GetUpgradeConfigs(), func(existing *envoyroutev3.RouteAction_UpgradeConfig) bool {
			return strings.EqualFold(existing.GetUpgradeType(), desired.GetUpgradeType())
		})
		config := proto.Clone(desired).(*envoyroutev3.RouteAction_UpgradeConfig)
		if idx >= 0 {
			action.UpgradeConfigs[idx] = config
			continue
		}
		action.UpgradeConfigs = append(action.UpgradeConfigs, config)
	}
}
