package options

type DiscoveryOptions struct {
	AutoDiscoverSwagger bool
	SwaggerUrisToTry    []string

	AutoDiscoverNATS      bool
	AutoDiscoverFaaS      bool
	AutoDiscoverFission   bool
	AutoDiscoverProjectFn bool
	ClusterIDsToTry       []string

	AutoDiscoverGRPC bool
}
