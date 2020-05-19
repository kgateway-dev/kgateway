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

	GlooeRateLimitConnectedState = "glooe_ratelimit_connected_state"
)

var (
	resourcesOutOfSyncMessage = func(resourceNames []string) string {
		return fmt.Sprintf("Gloo has detected that the data plane is out of sync. The following types of resources have not been accepted: %v. "+
			"Gloo will not be able to process any other configuration updates until these errors are resolved.", resourceNames)
	}
)

func ResourcesSyncedOverXds(stats, deploymentName string) bool {
	var outOfSyncResources []string
	metrics := parseMetrics(stats, []string{glooeTotalEntites, glooeInSyncEntities}, deploymentName)
	for metric, val := range metrics {
		if strings.HasPrefix(metric, glooeTotalEntites) {
			inSyncMetric := strings.ReplaceAll(metric, glooeTotalEntites, glooeInSyncEntities)
			if inSyncVal, ok := metrics[inSyncMetric]; ok {
				if val > inSyncVal {
					outOfSyncResources = append(outOfSyncResources, strings.ReplaceAll(metric, glooeTotalEntites, ""))
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

func RateLimitIsConnected(stats string) bool {
	connectedStateErrMessage := "The rate limit server is out of sync with the Gloo control plane and is not receiving valid gloo config.\n" +
		"You may want to try using the `glooctl debug logs --errors-only` command to find any relevant error logs."

	metrics := parseMetrics(stats, []string{ GlooeRateLimitConnectedState }, "gloo")

	if val, ok := metrics[GlooeRateLimitConnectedState]; ok && val == 0 {
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
			if !RateLimitIsConnected(stats) {
				return false, nil
			}
			fmt.Printf("OK\n")
		}
	}

	return true, nil
}
