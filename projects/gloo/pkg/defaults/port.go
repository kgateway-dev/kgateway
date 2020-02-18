package defaults

import "time"

var HttpPort uint32 = 8080
var HttpsPort uint32 = 8443
var EnvoyAdminPort uint32 = 19000
var GlooXdsPort = 9977
var GlooValidationPort = 9988
var GlooMtlsModeXdsPort = 9999
var DefaultRefreshRate = time.Minute

// Used for testing
var TcpPort uint32 = 8000
