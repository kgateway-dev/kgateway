package check

import (
	"context"
	"fmt"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	v1 "k8s.io/api/apps/v1"
	"strconv"
	"strings"
)

func checkRateLimitConnectedState(stats string, deploymentName string, genericErrMessage string, connectedStateErrMessage string) bool {

	if strings.TrimSpace(stats) == "" {
		fmt.Println(genericErrMessage+": could not find any metrics at", promStatsPath, "endpoint of the "+deploymentName+" deployment")
		return false
	}

	if !strings.Contains(stats, "glooe_ratelimit_connected_state 1") {
		fmt.Println(connectedStateErrMessage)
		return false
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
			"You may want to try using the `glooctl debug logs --errors-only` command to find relevant error logs.") {
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