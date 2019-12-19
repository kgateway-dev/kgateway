package upgradeconfig

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

const (
	WebSocketUpgradeType = "websocket"
)

func ValidateHCMUpgradeConfigs(upgradeConfigs []*envoyhttp.HttpConnectionManager_UpgradeConfig) error {
	uniqConfigs := map[string]bool{}
	var multiErr *multierror.Error

	for _, config := range upgradeConfigs {
		if _, ok := uniqConfigs[config.UpgradeType]; ok {
			multiErr = multierror.Append(multiErr, errors.Errorf("upgrade config %s is not unique", config.UpgradeType))
		}
		uniqConfigs[config.UpgradeType] = true
	}
	return multiErr.ErrorOrNil()
}

func ValidateRouteUpgradeConfigs(upgradeConfigs []*envoyroute.RouteAction_UpgradeConfig) error {
	uniqConfigs := map[string]bool{}
	var multiErr *multierror.Error

	for _, config := range upgradeConfigs {
		if _, ok := uniqConfigs[config.UpgradeType]; ok {
			multiErr = multierror.Append(multiErr, errors.Errorf("upgrade config %s is not unique", config.UpgradeType))
		}
		uniqConfigs[config.UpgradeType] = true
	}
	return multiErr.ErrorOrNil()
}
