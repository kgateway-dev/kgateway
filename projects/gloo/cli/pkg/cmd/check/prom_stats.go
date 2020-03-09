package check

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

const promStatsPath = "/stats/prometheus"

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

func checkProxyUpdate(stats string, localPort string, errMessage string) (bool, error) {

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
	desiredMetricsSegments := []string{"update_attempt", "update_rejected", "update_failure"}
	statsMap := parseMetrics(stats, desiredMetricsSegments)
	newStatsMap := parseMetrics(newStats, desiredMetricsSegments)

	if reflect.DeepEqual(newStatsMap, statsMap) {
		fmt.Printf("OK\n")
		return true, nil
	}

	for k, oldVal := range statsMap {
		if newVal, ok := newStatsMap[k]; ok && strings.Contains(k, "attempt") && newVal > oldVal {
			// at least one attempt for this counter- check if any were rejected or failed
			rejectedMetric := strings.Replace(k, "attempt", "rejected", -1)
			newRejected, newOk := newStatsMap[rejectedMetric]
			oldRejected, oldOk := statsMap[rejectedMetric]
			// for example, if envoy_http_rds_update_rejected{envoy_http_conn_manager_prefix="http",envoy_rds_route_config="listener-__-8080-routes"}
			// increases, which occurs if envoy cannot parse the config from gloo
			if newOk && oldOk && newRejected > oldRejected {
				fmt.Printf("An update to your gateway-proxy deployment was rejected due to schema/validation errors. The %v metric increased.\n"+
					"You may want to try using the `glooctl proxy logs` or `glooctl debug logs` commands.\n", rejectedMetric)
				return false, nil
			}
			failureMetric := strings.Replace(k, "attempt", "failure", -1)
			newFailure, newOk := newStatsMap[failureMetric]
			oldFailure, oldOk := statsMap[failureMetric]
			if newOk && oldOk && newFailure > oldFailure {
				fmt.Printf("An update to your gateway-proxy deployment was rejected due to network errors. The %v metric increased.\n"+
					"You may want to try using the `glooctl proxy logs` or `glooctl debug logs` commands.\n", failureMetric)
				return false, nil
			}
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
		if desiredMetric {
			pieces := strings.Fields(trimLine) // split by white spaces
			metric := strings.Join(pieces[0:len(pieces)-1], "")
			metricVal, err := strconv.Atoi(pieces[len(pieces)-1])
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
