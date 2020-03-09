package check

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

const promStatsPath = "/stats/prometheus"
const metricsUpdateInterval = time.Millisecond * 250

func checkProxyPromStats(ctx context.Context, glooNamespace string) (bool, error) {

	// check if any proxy instances are out of sync with the Gloo control plane
	errMessage := "Problem while checking for out of sync proxies"

	// port-forward gateway-proxy deployment and get prometheus metrics
	freePort, err := cliutil.GetFreePort()
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	localPort := strconv.Itoa(freePort)
	adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
	// stats is the string containing all stats from /stats/prometheus
	stats, portFwdCmd, err := cliutil.PortForwardGet(ctx, glooNamespace, "deploy/gateway-proxy",
		localPort, adminPort, false, promStatsPath)
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	defer portFwdCmd.Process.Release()
	defer portFwdCmd.Process.Kill()

	if !checkProxyConnectedState(stats, errMessage,
		"Your gateway-proxy is out of sync with the Gloo control plane and is not receiving valid gloo config.\n"+
			"You may want to try looking at your gloo or gateway-proxy logs or using the `glooctl debug log` command.") {
		return false, nil
	}

	return checkProxyUpdate(stats, localPort, errMessage)
}

// checks that envoy_control_plane_connected_state metric has a value of 1
func checkProxyConnectedState(stats string, genericErrMessage string, connectedStateErrMessage string) bool {

	if strings.TrimSpace(stats) == "" {
		fmt.Println(genericErrMessage+": could not find any metrics at", promStatsPath, "endpoint of the gateway-proxy deployment")
		return false
	}

	if !strings.Contains(stats, "envoy_control_plane_connected_state{} 1") {
		fmt.Println(connectedStateErrMessage)
		return false
	}

	return true
}

// checks that update_rejected and update_failure stats have not increased by getting stats from /stats/prometheus again
func checkProxyUpdate(stats string, localPort string, errMessage string) (bool, error) {

	// wait for metrics to update
	time.Sleep(metricsUpdateInterval)

	// gather metrics again
	res, err := http.Get("http://localhost:" + localPort + promStatsPath)
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	if res.StatusCode != 200 {
		fmt.Println(errMessage+": received unexpected status code", res.StatusCode, "from", promStatsPath, "endpoint of the gateway-proxy deployment")
		return false, nil
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(errMessage)
		return false, err
	}
	res.Body.Close()
	newStats := string(b)

	if strings.TrimSpace(newStats) == "" {
		fmt.Println(errMessage+": could not find any metrics at", promStatsPath, "endpoint of the gateway-proxy deployment")
		return false, nil
	}

	// for example, look for stats like "envoy_http_rds_update_attempt" and "envoy_http_rds_update_rejected"
	// more info at https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/mgmt_server#xds-subscription-statistics
	desiredMetricsSegments := []string{"update_rejected", "update_failure"}
	statsMap := parseMetrics(stats, desiredMetricsSegments)
	newStatsMap := parseMetrics(newStats, desiredMetricsSegments)

	if reflect.DeepEqual(newStatsMap, statsMap) {
		fmt.Printf("OK\n")
		return true, nil
	}

	for metricName, oldVal := range statsMap {
		newVal, ok := newStatsMap[metricName]
		if ok && strings.Contains(metricName, "rejected") && newVal > oldVal {
			// for example, if envoy_http_rds_update_rejected{envoy_http_conn_manager_prefix="http",envoy_rds_route_config="listener-__-8080-routes"}
			// increases, which occurs if envoy cannot parse the config from gloo
			fmt.Printf("An update to your gateway-proxy deployment was rejected due to schema/validation errors. The %v metric increased.\n"+
				"You may want to try using the `glooctl proxy logs` or `glooctl debug logs` commands.\n", metricName)
			return false, nil
		} else if ok && strings.Contains(metricName, "failure") && newVal > oldVal {
			fmt.Printf("An update to your gateway-proxy deployment was rejected due to network errors. The %v metric increased.\n"+
				"You may want to try using the `glooctl proxy logs` or `glooctl debug logs` commands.\n", metricName)
			return false, nil
		}
	}

	fmt.Printf("OK\n")
	return true, nil
}

// parseMetrics parses prometheus metrics and returns a map from the metric name and labels to its value.
// It expects to only look for int values!
func parseMetrics(stats string, desiredMetricSegments []string) map[string]int {
	statsMap := make(map[string]int)
	statsLines := strings.Split(stats, "\n")
	for _, line := range statsLines {
		trimLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimLine, "#") || trimLine == "" {
			continue // Ignore comments, help text, type info, empty lines (https://github.com/prometheus/docs/blob/master/content/docs/instrumenting/exposition_formats.md#comments-help-text-and-type-information)
		}
		desiredMetric := false
		for _, s := range desiredMetricSegments {
			if strings.Contains(trimLine, s) {
				desiredMetric = true
			}
		}
		// sample line: "envoy_http_rds_update_rejected{envoy_http_conn_manager_prefix="http",envoy_rds_route_config="listener-__-8080-routes"} 90"
		if desiredMetric {
			pieces := strings.Fields(trimLine)                    // split by white spaces
			metric := strings.Join(pieces[0:len(pieces)-1], "")   // get all but last piece (as one string)- this is metric name and labels
			metricVal, err := strconv.Atoi(pieces[len(pieces)-1]) // get last piece (as int)- this is metric value
			if err != nil {
				fmt.Printf("Found an unexpected format in metrics at %v endpoint of the gateway-proxy deployment. "+
					"Expected %v metric to have an int value but got value %v.\nContinuing check...", promStatsPath, metric, pieces[len(pieces)-1])
				continue
			}
			statsMap[metric] = metricVal
		}
	}
	return statsMap
}
