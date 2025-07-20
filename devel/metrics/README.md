# Working With Metrics

## Metrics Package
The [metrics](/pkg/metrics/metrics.go) package provides constructors to create metric recorders:
* `NewCounter(opts CounterOpts, labels []string) Counter`
* `NewHistogram(opts HistogramOpts, labels []string) Histogram`
* `NewGauge(opts GaugeOpts, labels []string) Gauge`

These contructors handle registering the metrics.

The underlying implementation is based on [github.com/prometheus/client_golang/prometheus](github.com/prometheus/client_golang/prometheus).

### Best practices and common patterns
* Metrics are expected to have a namespace and subdomain defined in their options
  * The default namespace of "kgateway" will be used if no namespace is provided. This will likely be the correct namespace.
* When passing labels to methods such as `Add(...)` or `Set(...)`, consider creating a struct to hold the label values with a method to convert it into a slice of Labels. This improves readability and ensures that any missed labels are present with a default ("") value.
  * See `GatewayResourceMetricLabels` in [/internal/kgateway/krtcollections/metrics.go](/internal/kgateway/krtcollections/metrics.go) for an example
* Follow the [Prometheus Metric and Label Naming Guide](https://prometheus.io/docs/practices/naming/) when possible
  * promlinter is now used in static code analysis to validate metric names, types, and metadata
* The metrics package supports an `Active() bool` method with the underlying value evaluated at startup, and can not be meaningfully changed during execution.
  * In a test context, the value defaults to `true` and can be set with `metrics.SetActive(bool)`

## Metric collection packages
Several packages have interfaces created to standardize collection of metrics around existing frameworks
* [CollectionMetricsRecorder](/internal/kgateway/krtcollections/metrics.go) for [/internal/kgateway/krtcollections](/internal/kgateway/krtcollections/)
  * Created by `NewCollectionMetricsRecorder(collectionName string) CollectionMetricsRecorder`
* [controllerMetricsRecorder](/internal/kgateway/controller/metrics.go) for [/internal/kgateway/controller](/internal/kgateway/controller/)
  * Created by `newControllerMetricsRecorder(controllerName string) controllerMetricsRecorder `
* [TranslatorMetricsRecorder](/internal/kgateway/translator/metrics/metrics.go) for [/internal/kgateway/translator/](/internal/kgateway/translator/)
  * Created by ` NewTranslatorMetricsRecorder(translatorName string) TranslatorMetricsRecorder`
* [statusSyncMetricsRecorder](/internal/kgateway/proxy_syncer/metrics.go) for the status syncer in [/internal/kgateway/proxy_syncer/](/internal/kgateway/proxy_syncer/)
  * Created by `NewStatusSyncMetricsRecorder(syncerName string) statusSyncMetricsRecorder`

Objects returned from these constructors will be unique, but the underlying metrics will be shared.

These objects all support a `Start` method, that can be placed at the beginning of processing an event:
```
	metricsRecorder := NewCollectionMetricsRecorder("Gateways")

	h.Gateways = krt.NewCollection(gws, func(kctx krt.HandlerContext, i *gwv1.Gateway) *ir.Gateway {
		defer metricsRecorder.TransformStart()(nil)

		....
	}
```

These `Start` methods return a function to be defered to run on completion of the event handling, allowing collection of timing and other metrics. If the `Start` method is not called, those metrics will not be collected, but there will be no failures.


### Gathering metrics from a KRT collection
Metric gathering capabilities can be added to KRT collections by a couple of methods
* In the transform function
As in the example above, when a collection is created with a transform function, metric handling code can be added to the handler function. This code will be run every time a member of the collection is transformed.
* With `RegisterEvents[T any](c krt.Collection[T], f func(o krt.Event[T]))`
Event handlers can be registered for KRT collections for metrics that need to be updated on Add, Delete, and/or Update. `RegisterEvents` is a helper function that will register the passed function as an event handler of the collection. This code will run when a
KRT collection is modified. Example:
```
	tcproutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](istioClient, gvr.TCPRoute, kubetypes.StandardInformer, filter), krtopts.ToOptions("TCPRoute")...)
	metrics.RegisterEvents(tcproutes, func(o krt.Event[*gwv1a2.TCPRoute]) {
		gwResourceMetricEventHandler(o, "TCPRoute")
	})
```

Both approaches may be used with a single collection.
