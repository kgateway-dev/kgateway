//go:build ignore

package helpers

import (
	"fmt"

	"go.opencensus.io/stats/view"
)

// ReadMetricByLabel looks up the specified metricName and returns the latest data
// recorded for the time series with the specified label key/value pair.
//
// If the metric has not yet been registered, this function will fail. If the metric
// has been registered, but there is not yet any time series data recorded with the label
// key/value provided, then an error is returned. The error response allows tests to distinguish
// "the metric was never recorded" from "a value of 0 was recorded"
func ReadMetricByLabel(metricName string, labelKey string, labelValue string) (int, error) {
	rows, err := view.RetrieveData(metricName)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve data for %s: %w", metricName, err)
	}
	for _, row := range rows {
		for _, tag := range row.Tags {
			if tag.Key.Name() == labelKey && tag.Value == labelValue {
				return int(row.Data.(*view.LastValueData).Value), nil
			}
		}
	}
	return 0, fmt.Errorf("%s does not have any time series with label (key=%s,value=%s)", metricName, labelKey, labelValue)
}
