package metricsservice

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	envoymet "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v2"
	"github.com/solo-io/go-utils/contextutils"
	prometheus "istio.io/gogo-genproto/prometheus"
)

type MetricsHandler interface {
	HandleMetrics(context.Context, *envoymet.StreamMetricsMessage) error
}

func NewDefaultMetricsHandler(storage StorageClient, merger *UsageMerger) *metricsHandler {
	return &metricsHandler{
		storage:     storage,
		usageMerger: merger,
	}
}

// get the default metrics handler backed by a config map
func NewConfigMapBackedDefaultHandler(ctx context.Context) (*metricsHandler, error) {
	ns := os.Getenv("POD_NAMESPACE")
	storageClient, err := NewDefaultConfigMapStorage(ns)
	if err != nil {
		contextutils.LoggerFrom(ctx).Errorw("err starting up metrics watcher", zap.Error(err))
		return nil, err
	}

	merger := NewUsageMerger(time.Now)

	return NewDefaultMetricsHandler(storageClient, merger), nil
}

type metricsHandler struct {
	storage     StorageClient
	usageMerger *UsageMerger
}

func (m *metricsHandler) HandleMetrics(ctx context.Context, met *envoymet.StreamMetricsMessage) error {
	newMetrics, err := buildNewMetrics(ctx, met)
	if err != nil {
		contextutils.LoggerFrom(ctx).Errorf("Error while building new usage: %s", err.Error())
		return err
	}

	existingUsage, err := m.storage.GetUsage(ctx)
	if err != nil {
		contextutils.LoggerFrom(ctx).Errorf("Error while retrieving old usage: %s", err.Error())
		return err
	}

	mergedUsage := m.usageMerger.MergeUsage(met.Identifier.Node.Id, existingUsage, newMetrics)

	err = m.storage.RecordUsage(ctx, mergedUsage)
	if err != nil {
		contextutils.LoggerFrom(ctx).Errorf("Error while storing new usage: %s", err.Error())
		return err
	}
	return nil
}

func buildNewMetrics(ctx context.Context, metricsMessage *envoymet.StreamMetricsMessage) (*EnvoyMetrics, error) {
	newMetricsEntry := &EnvoyMetrics{}

	for _, v := range metricsMessage.EnvoyMetrics {
		name := v.GetName()
		switch {
		// ignore cluster-specific stats, like accesses to the admin port cluster
		case strings.HasPrefix(name, "cluster") || strings.HasPrefix(name, HttpStatPrefix+".admin"):
			continue
		// ignore the static listeners that we explicitly create
		case strings.HasPrefix(name, HttpStatPrefix+".prometheus") || strings.HasPrefix(name, HttpStatPrefix+".read_config"):
			continue
		case strings.HasPrefix(name, HttpStatPrefix) && strings.HasSuffix(name, "downstream_rq_total"):
			newMetricsEntry.HttpRequests += sumMetricCounter(v.Metric)
		case strings.HasPrefix(name, TcpStatPrefix) && strings.HasSuffix(name, "downstream_cx_total"):
			newMetricsEntry.TcpConnections += sumMetricCounter(v.Metric)
		case v.GetName() == ServerUptime:
			uptime := sumMetricGauge(v.Metric)
			uptimeDuration, err := time.ParseDuration(fmt.Sprintf("%ds", uptime))
			if err != nil {
				return nil, err
			}
			newMetricsEntry.Uptime = uptimeDuration
		}
	}

	return newMetricsEntry, nil
}

func sumMetricCounter(metrics []*prometheus.Metric) uint64 {
	var sum uint64
	for _, m := range metrics {
		sum += uint64(m.Counter.Value)
	}

	return sum
}

func sumMetricGauge(metrics []*prometheus.Metric) uint64 {
	var sum uint64
	for _, m := range metrics {
		sum += uint64(m.Gauge.Value)
	}

	return sum
}
