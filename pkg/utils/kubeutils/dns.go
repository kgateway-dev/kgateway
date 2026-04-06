package kubeutils

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	maxLabelLength = 63
	truncateLength = 50
	hashLength     = 12
)

// ServiceFQDN returns the FQDN for the Service, assuming it is being accessed from within the Cluster
func ServiceFQDN(serviceMeta metav1.ObjectMeta) string {
	return GetServiceHostname(serviceMeta.Name, serviceMeta.Namespace)
}

// SafeGatewayLabelValue converts a gateway name to a value that is RFC1123 label compliant.
// If the name is longer than 63 characters, it truncates it to 50 characters and adds a SHA256 hash suffix.
//
// This must stay in sync with the Helm template kgateway.gateway.safeLabelValue
// defined in pkg/kgateway/helm/envoy/templates/_helpers.tpl.
func SafeGatewayLabelValue(name string) string {
	if len(name) <= maxLabelLength {
		return name
	}
	hash := sha256.Sum256([]byte(name))
	hashStr := hex.EncodeToString(hash[:])[:hashLength]
	truncated := strings.TrimSuffix(name[:truncateLength], "-")
	return truncated + "-" + hashStr
}
