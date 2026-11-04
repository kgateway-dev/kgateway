package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/sds"
)

// defaultSDSAddress matches pkg/sds.Config's default SdsServerAddress. Used
// as a fallback for the healthcheck subcommand below when SDS_SERVER_ADDRESS
// is unset.
const defaultSDSAddress = "127.0.0.1:8234"

// healthCheckTimeout bounds the readiness-probe dial.
const healthCheckTimeout = time.Second

func main() {
	// The "healthcheck" subcommand is used by the sds container's readiness
	// probe. SDS binds to loopback, so a kubelet tcpSocket probe - which dials
	// the pod IP - can't reach it; instead the probe execs this binary inside
	// the pod and dials loopback directly.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		runHealthCheck()
		return
	}

	sds.RunMain()
}

// runHealthCheck dials the SDS server over loopback and exits non-zero if it
// is not accepting connections.
func runHealthCheck() {
	addr := healthCheckAddress(os.Getenv("SDS_SERVER_ADDRESS"))
	conn, err := net.DialTimeout("tcp", addr, healthCheckTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sds healthcheck: %v\n", err)
		os.Exit(1)
	}
	_ = conn.Close()
}

// healthCheckAddress returns the loopback address to probe. SDS always
// serves Envoy over loopback, so we dial 127.0.0.1 using the port from the
// configured bind address (env), falling back to the default. Resolving
// against loopback keeps the probe correct even if the bind address is a
// wildcard (e.g. 0.0.0.0:8234) or unset.
func healthCheckAddress(env string) string {
	if env == "" {
		env = defaultSDSAddress
	}
	_, port, err := net.SplitHostPort(env)
	if err != nil {
		// Not host:port (e.g. just a port or malformed); fall back to default port.
		_, port, _ = net.SplitHostPort(defaultSDSAddress)
	}
	return net.JoinHostPort("127.0.0.1", port)
}
