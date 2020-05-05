package check

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	v1 "k8s.io/api/apps/v1"
)

const (
	glooRateLimitConnectedState = "glooe_ratelimit_connected_state"
	glooStatsPath               = "/metrics"
)

func checkRateLimitConnectedState(stats string, deploymentName string, genericErrMessage string, connectedStateErrMessage string) bool {

	if strings.TrimSpace(stats) == "" {
		fmt.Println(genericErrMessage+": could not find any metrics at", glooStatsPath, "endpoint of the "+deploymentName+" deployment")
		return false
	}

	// We'll just look for the presence of the stat that indicates an error. Since this stat was just introduced, we
	// don't want `glooctl check` to start failing when a user upgrades glooctl but not gloo
	if strings.Contains(stats, "glooe_ratelimit_connected_state 0") {
		fmt.Println(connectedStateErrMessage)
		return false
	}

	totalMetric := "glooe_solo_io_xds_total_entities{resource=\"type.googleapis.com/glooe.solo.io.RateLimitConfig\"}"
	inSyncMetric := "glooe_solo_io_xds_insync{resource=\"type.googleapis.com/glooe.solo.io.RateLimitConfig\"}"
	metrics := parseMetrics(stats, []string{totalMetric, inSyncMetric}, deploymentName)

	if totalCount, ok := metrics[totalMetric]; ok {
		if inSyncCount, ok := metrics[inSyncMetric]; ok {
			if totalCount > inSyncCount {
				fmt.Println(connectedStateErrMessage)
				return false
			}
		}
	}

	return true
}

func checkGlooRateLimitPromStats(ctx context.Context, glooNamespace string, deploymentName string) (bool, error) {
	errMessage := "Problem while checking for out of sync rate limit server"

	// port-forward proxy deployment and get prometheus metrics
	freePort, err := cliutil.GetFreePort()
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	localPort := strconv.Itoa(freePort)
	adminPort := strconv.Itoa(int(defaults.GlooAdminPort))
	// stats is the string containing all stats from /stats/prometheus
	stats, portFwdCmd, err := cliutil.PortForwardGet(ctx, glooNamespace, "deploy/"+deploymentName,
		localPort, adminPort, false, glooStatsPath)
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	if portFwdCmd.Process != nil {
		defer portFwdCmd.Process.Release()
		defer portFwdCmd.Process.Kill()
	}

	if !checkRateLimitConnectedState(stats, deploymentName, errMessage,
		"The rate limit server is out of sync with the Gloo control plane and is not receiving valid gloo config.\n"+
			"You may want to try using the `glooctl debug logs --errors-only` command to find any relevant error logs.") {
		return false, nil
	}

	return true, nil
}

func checkEnterprisePromStats(ctx context.Context, glooNamespace string, deployments *v1.DeploymentList) (bool, error) {
	for _, deployment := range deployments.Items {
		if deployment.Name == "rate-limit" {
			fmt.Printf("Checking rate limit server... ")
			if passed, err := checkGlooRateLimitPromStats(ctx, glooNamespace, "gloo"); !passed || err != nil {
				return passed, err
			}
			fmt.Printf("OK\n")
		}
	}

	return true, nil
}
