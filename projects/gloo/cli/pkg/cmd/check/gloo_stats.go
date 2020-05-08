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
	glooDeployment      = "gloo"
	rateLimitDeployment = "rate-limit"
	glooStatsPath       = "/metrics"

	glooeTotalEntites   = "glooe_solo_io_xds_total_entities"
	glooeInSyncEntities = "glooe_solo_io_xds_insync"

	GlooeRateLimitDisconnected = "glooe_ratelimit_connected_state 0"
)

var (
	resourceNames = []string{
		"type.googleapis.com/enterprise.gloo.solo.io.ExtAuthConfig",
		"type.googleapis.com/envoy.api.v2.Cluster",
		"type.googleapis.com/envoy.api.v2.ClusterLoadAssignment",
		"type.googleapis.com/envoy.api.v2.Listener",
		"type.googleapis.com/envoy.api.v2.RouteConfiguration",
		"type.googleapis.com/glooe.solo.io.RateLimitConfig",
	}

	resourcesOutOfSyncMessage = func(resourceNames []string) string {
		return fmt.Sprintf("Gloo has detected that the data plane is out of sync. The following types of resources have not been accepted: %v. "+
			"Gloo will not be able to process any other configuration updates until these errors are resolved.", resourceNames)
	}
)

func ResourcesSyncedOverXds(stats, deploymentName string) bool {
	var outOfSyncResources []string
	for _, resourceName := range resourceNames {
		totalMetric := fmt.Sprintf(`%s{resource="%s"}`, glooeTotalEntites, resourceName)
		inSyncMetric := fmt.Sprintf(`%s{resource="%s"}`, glooeInSyncEntities, resourceName)
		metrics := parseMetrics(stats, []string{totalMetric, inSyncMetric}, deploymentName)

		if totalCount, ok := metrics[totalMetric]; ok {
			if inSyncCount, ok := metrics[inSyncMetric]; ok {
				if totalCount > inSyncCount {
					outOfSyncResources = append(outOfSyncResources, resourceName)
				}
			}
		}
	}
	if len(outOfSyncResources) > 0 {
		fmt.Println(resourcesOutOfSyncMessage(outOfSyncResources))
		return false
	}

	return true
}

func CheckRateLimitConnectedState(stats string) bool {
	connectedStateErrMessage := "The rate limit server is out of sync with the Gloo control plane and is not receiving valid gloo config.\n" +
		"You may want to try using the `glooctl debug logs --errors-only` command to find any relevant error logs."

	// glooe publishes this stat when it detects an error in the config
	if strings.Contains(stats, GlooeRateLimitDisconnected) {
		fmt.Println(connectedStateErrMessage)
		return false
	}

	return true
}

func checkGlooePromStats(ctx context.Context, glooNamespace string, deployments *v1.DeploymentList) (bool, error) {
	errMessage := "Problem while checking for gloo xds errors"

	// port-forward proxy deployment and get prometheus metrics
	freePort, err := cliutil.GetFreePort()
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	localPort := strconv.Itoa(freePort)
	adminPort := strconv.Itoa(int(defaults.GlooAdminPort))
	// stats is the string containing all stats from /stats/prometheus
	stats, portFwdCmd, err := cliutil.PortForwardGet(ctx, glooNamespace, "deploy/"+glooDeployment,
		localPort, adminPort, false, glooStatsPath)
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	if portFwdCmd.Process != nil {
		defer portFwdCmd.Process.Release()
		defer portFwdCmd.Process.Kill()
	}

	if strings.TrimSpace(stats) == "" {
		fmt.Println(errMessage+": could not find any metrics at", glooStatsPath, "endpoint of the "+glooDeployment+" deployment")
		return false, nil
	}

	if !ResourcesSyncedOverXds(stats, glooDeployment) {
		return false, nil
	}

	for _, deployment := range deployments.Items {
		if deployment.Name == rateLimitDeployment {
			fmt.Printf("Checking rate limit server... ")
			if !CheckRateLimitConnectedState(stats) {
				return false, nil
			}
			fmt.Printf("OK\n")
		}
	}

	return true, nil
}