//go:build e2e

package udproute

import (
	"path/filepath"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	// ctxTimeout bounds the whole suite, timeout bounds individual assertions.
	ctxTimeout = 10 * time.Minute
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

	// Weighted multi-backend case. Backend A has weight 80, backend B weight 20.
	multiNs        = "udp-multi"
	multiGwName    = "udp-multi-gw"
	multiRouteName = "udp-multi-route"
	multiListener  = "udp"
	multiAnswerA   = "10.1.1.1"
	multiAnswerB   = "10.2.2.2"
	// multiDeploymentB is backend B's Deployment, whose pods carry app=<name>.
	multiDeploymentB = "coredns-b"

	// Invalid-backend case. One valid backend (weight 20) and one nonexistent backend (weight 80).
	// The gateway must drop the 80% share, not give it to the valid backend.
	dropNs          = "udp-drop"
	dropGwName      = "udp-drop-gw"
	dropRouteName   = "udp-drop-route"
	dropListener    = "udp"
	dropValidAnswer = "10.1.1.1"
)

var (
	singleBackendManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "single-backend.yaml")
	multiBackendManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multi-backend.yaml")
	invalidBackendManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-backend.yaml")
)
