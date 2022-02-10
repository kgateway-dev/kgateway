package defaults

import "time"

const GlooRestXdsName = "rest_xds_cluster"

var HttpPort uint32 = 8080
var HttpsPort uint32 = 8443
var EnvoyAdminPort uint32 = 19000
var GlooAdminPort uint32 = 9091
var GlooRestXdsPort = 9976
var GlooXdsPort = 9977
var GlooValidationPort = 9988
var GlooMtlsModeRestXdsPort = 9998
var GlooMtlsModeXdsPort = 9999
var DefaultRefreshRate = time.Minute

// Used for testing
var TcpPort uint32 = 8000
var HybridPort uint32 = 8082
