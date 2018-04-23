package options

type DiscoveryOptions struct {
	AutoDiscoverSwagger bool
	SwaggerUrisToTry    []string

	AutoDiscoverNATS    bool
	AutoDiscoverFAAS    bool
	AutoDiscoverFission bool
	ClusterIDsToTry     []string

	AutoDiscoverGRPC bool
}
