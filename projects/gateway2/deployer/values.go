package deployer

// The top-level helm values used by the deployer.
type helmConfig struct {
	Gateway *helmGateway `json:"gateway,omitempty"`
}

type helmGateway struct {
	// naming
	Name             *string `json:"name,omitempty"`
	GatewayName      *string `json:"gatewayName,omitempty"`
	NameOverride     *string `json:"nameOverride,omitempty"`
	FullnameOverride *string `json:"fullnameOverride,omitempty"`

	// deployment/service values
	Ports []helmPort `json:"ports,omitempty"`

	// envoy container values
	Image *helmImage `json:"image,omitempty"`

	// istio values
	IstioSDS *helmIstioSds `json:"istioSDS,omitempty"`

	// xds values
	Xds *helmXds `json:"xds,omitempty"`
}

// helmPort represents a Gateway Listener port
type helmPort struct {
	Port       *uint16 `json:"port,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
	Name       *string `json:"name,omitempty"`
	TargetPort *uint16 `json:"targetPort,omitempty"`
}

type helmImage struct {
	Registry   *string `json:"registry,omitempty"`
	Repository *string `json:"repository,omitempty"`
	Tag        *string `json:"tag,omitempty"`
	Digest     *string `json:"digest,omitempty"`
	PullPolicy *string `json:"pullPolicy,omitempty"`
}

// helmXds represents the xds host and port to which envoy will connect
// to receive xds config updates
type helmXds struct {
	Host *string `json:"host,omitempty"`
	Port *int32  `json:"port,omitempty"`
}

type helmIstioSds struct {
	Enabled *bool `json:"enabled,omitempty"`
}
