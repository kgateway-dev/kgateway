package constants

const (
	ConsulEndpointMetadataMatchTrue  = "1"
	ConsulEndpointMetadataMatchFalse = "0"

	// We use these prefixes to avoid shadowing in case a data center name is the same as a tag name
	ConsulTagKeyPrefix        = "tag_"
	ConsulDataCenterKeyPrefix = "dc_"
)
