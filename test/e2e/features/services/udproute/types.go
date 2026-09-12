//go:build e2e

package udproute

import (
	"path/filepath"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	// ctxTimeout bounds the whole suite; timeout bounds individual assertions.
	ctxTimeout = 5 * time.Minute
	timeout    = 60 * time.Second

	// clientPodName is the in-cluster dnsutils pod the suite execs `dig` from.
	clientPodName = "dns-client"
	// queryName is the DNS name the CoreDNS backends answer with a distinct A record.
	queryName = "foo.bar.com"
	// udpListenerPort is the Gateway UDP listener port dig targets.
	udpListenerPort = 5300

	// Single-backend case.
	singleNs        = "udp-single"
	singleGwName    = "udp-single-gw"
	singleRouteName = "udp-single-route"
	singleListener  = "udp"
	singleAnswerIP  = "10.1.1.1"

	// Weighted multi-backend case: backend A has weight 80, backend B weight 20.
	multiNs        = "udp-multi"
	multiGwName    = "udp-multi-gw"
	multiRouteName = "udp-multi-route"
	multiListener  = "udp"
	multiAnswerA   = "10.1.1.1"
	multiAnswerB   = "10.2.2.2"
)

var (
	singleBackendManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-backend.yaml")
	multiBackendManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-backend.yaml")
)
