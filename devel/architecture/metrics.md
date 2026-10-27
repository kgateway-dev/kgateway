# Working With Metrics

## Metrics Package
The [metrics](/pkg/metrics/metrics.go) package provides constructors to create metric recorders:
* `NewCounter(opts CounterOpts, labels []string) Counter`
* `NewHistogram(opts HistogramOpts, labels []string) Histogram`
* `NewGauge(opts GaugeOpts, labels []string) Gauge`

These constructors handle registering the metrics with a metrics registry. By default, a new empty registry is created at startup. This can be replaced with a custom registry, or a built-in registry used by controller-runtime can be enabled by using:
* `SetRegistry(useBuiltinRegistry bool, r RegistererGatherer)`

The underlying implementation is based on [github.com/prometheus/client_golang/prometheus](github.com/prometheus/client_golang/prometheus).

### Best practices and common patterns
* Metrics are expected to have a namespace and subsystem defined in their options
  * The default namespace of "kgateway" will be used if no namespace is provided. This will likely be the correct namespace.
* When passing labels to methods such as `Add(...)` or `Set(...)`, consider creating a struct to hold the label values with a method to convert it into a slice of Labels. This improves readability and ensures that any missed labels are present with a default ("") value.
  * See [`resourceMetricLabels`](/pkg/krtcollections/metrics/metrics.go) for an example.
* Follow the [Prometheus Metric and Label Naming Guide](https://prometheus.io/docs/practices/naming/) when possible.
  * `promlinter` is now used in static code analysis to validate metric names, types, and metadata.
* The metrics package supports an `Active() bool` method with the underlying value evaluated at startup, and can not be meaningfully changed during execution.
  * In a test context, the value defaults to `true` and can be set with `metrics.SetActive(bool)`

## Metric collection packages
Several packages have interfaces created to standardize collection of metrics around existing frameworks
* [CollectTranslationMetrics](/pkg/kgateway/translator/metrics/metrics.go) for [/pkg/kgateway/translator/](/pkg/kgateway/translator/)
  * Called with `CollectTranslationMetrics(labels TranslatorMetricLabels) func(error)` at the start of a translation function; the returned function is deferred to complete recording
* [StartResourceXDSSync and EndResourceXDSSync](/pkg/krtcollections/metrics/metrics.go) are used to track the time from a Gateway change to its XDS snapshot.
  * Used with `StartResourceXDSSync(details ResourceSyncDetails)` and `EndResourceXDSSync(details ResourceSyncDetails)`
  * `StartResourceSyncMetricsProcessing(ctx context.Context)` must be called at process startup to handle processing of resource metrics.
* [statussync.Writer](/pkg/pluginsdk/statussync/writer.go) records `kgateway_status_writes_total{kind, result}` once per `ApplyStatus` call that had a status to write; writers get it without any wiring.

Objects returned from these constructors will be unique, but the underlying metrics will be shared.

These objects all support a "CollectMetrics()" method, that can be called at the beginning of processing an event, and which returns a finish function called at the end of event processing, which also optionally records if an error occurred:
```go
var rErr error

finishMetrics := metrics.CollectTranslationMetrics(metrics.TranslatorMetricLabels{
  Name:       "gateway.Name",
  Namespace:  "gateway.Namespace",
  Translator: "TranslateGateway",
})
defer func() {
  finishMetrics(rErr)
}()

rErr := DoSomeTranslation()
```

These `Start` methods return a function to be defered to run on completion of the event handling, allowing collection of timing and other metrics. If the `Start` method is not called, those metrics will not be collected, but there will be no failures.


### Gathering metrics from a KRT collection
* The [metrics](/pkg/metrics/metrics.go) package provides a function used to create metrics related to KRT collection events:
  * `RegisterEvents[T any](c krt.Collection[T], f func(o krt.Event[T])) krt.Syncer`
* Event handlers can be registered for KRT collections for metrics that need to be updated on Add, Delete, and/or Update. `RegisterEvents` is a helper function that will register the passed function as an event handler of the collection. This code will run when a
KRT collection is modified.
* Example:
```go
	tcproutes := krt.WrapClient(kclient.NewDelayedInformer[*gwv1a2.TCPRoute](istioClient, gvr.TCPRoute, kubetypes.StandardInformer, filter), krtopts.ToOptions("TCPRoute")...)
	metrics.RegisterEvents(tcproutes, func(o krt.Event[*gwv1a2.TCPRoute]) {
		MyEventHandler(o)
	})
```
* This is used along with a helper function to instrument several KRT collections during [setup](/pkg/pluginsdk/collections/setup.go):
```go
metrics.RegisterEvents(httpRoutes, kmetrics.GetResourceMetricEventHandler[*gwv1.HTTPRoute]())
```

### `kgateway_envoy_xds_nack_resends_suppressed_total` Metric

This counter records when a NACK matches the response sent on its stream and the
snapshot still has the rejected version, so kgateway suppresses the immediate
resend and waits for a snapshot change, or for a retry delay that starts at 1s
and doubles to 1m while the same version keeps being rejected; the retried
response, rejected again, counts again. The `type_url` label takes the same
values as on `kgateway_envoy_xds_rejects_total` and
`kgateway_envoy_xds_rejects_active` (the type URL without its
`type.googleapis.com/` prefix), so the series join on it. ACKs, uncorrelated
NACKs, and NACKs for an older snapshot do not increment it.

An increment indicates that kgateway delayed a resend of rejected configuration;
it does not mean the rejection was resolved or count every retry that would have
occurred without suppression. Use `kgateway_envoy_xds_rejects_active` to monitor
active rejections and the NACK logs to investigate their cause.

### `kgateway_resources_updates_dropped_total` Metric
* This metric is never emitted under normal operating circumstances.
* It counts the number of times the background processing of resources metrics had to drop an update because the channel buffer was full.
* This indicates the metrics system, and probably the gateway, is overloaded.
* Once updates have been dropped, `kgateway_resources_*` metrics are no longer valid, until the process has been restarted.
* Metrics subsystems other than `kgateway_resources_` are not affected.

## Per-client xDS publication metrics

The controller builds an xDS snapshot for each connected client and holds back a
publication that would point Envoy at a cluster it does not have yet. These
metrics show when that happens and how it ends. Each carries `gateway` and
`namespace` labels unless noted. `kgateway_xds_snapshot_deferred_clients` covers
the stage before them: a client with no snapshot yet.

A client is *warm* when its first listener request on a connection carries a
previously accepted version, a sign that it may still be serving that
configuration, and *cold* otherwise. The version does not prove that the
configuration is active.

These metrics describe when the controller installs snapshots in its xDS cache.
They do not show when Envoy receives, accepts or applies them.

### `kgateway_xds_snapshot_perclient_defers_total` Metric
* Counts per-client snapshots built while a referenced cluster was not ready, by
  `reason`:
  * `missing_clusters`: a route references a cluster absent from CDS.
  * `missing_endpoints`: the cluster is in CDS but its endpoints have not been
    derived yet. A cluster whose derived endpoints are empty is the backend's
    real state and does not count.
* A build with both kinds of gap counts once under each reason.
* Each increment means the publication was resolved at publish time, for example
  by carrying a missing cluster forward from the last published snapshot, or by
  holding a route change until the new cluster is ready.
* A sustained rate for one gateway means some referenced cluster is not becoming
  ready.

### `kgateway_xds_snapshot_perclient_withheld_clients` Metric
* Counts clients whose first publish on their current connection is withheld
  because referenced clusters are not ready, by `state`. Until it goes out, a new
  proxy has no listeners and is not Ready.
  * `within_budget`: waiting inside the first-publish budget
    (`KGW_PER_CLIENT_PUBLISH_BUDGET`), which starts with the client's first
    deferred build rather than at connect. A cold client is published when the
    budget ends, ready or not, so it only ever appears here.
  * `warm_withheld`: a warm client still withheld after the budget because
    referenced clusters are missing from CDS. Publishing would replace routes it
    may still be serving with routes to clusters it does not have. While
    withheld it receives no updates at all, including certificate rotations. It
    is released `KGW_PER_CLIENT_WARM_WITHHOLD_BOUND` after the budget ended, or
    never if the bound is 0 or negative.
* The states do not overlap; their sum is every client being held.
* With the budget at 0 or negative, nothing is counted here: a withheld client
  waits without a deadline and is visible only through
  `kgateway_xds_snapshot_perclient_defers_total`.
* A state's series is removed when it has no clients.

### `kgateway_xds_snapshot_perclient_bounded_publishes_total` Metric
* Counts withheld or held publications that a time bound released before every
  referenced cluster was ready, by `mode`:
  * `first_publish`: a client never published before started on incomplete but
    consistent config when the first-publish budget ended, instead of waiting
    indefinitely.
  * `warm_truth`: a warm client whose only gaps were clusters with no derived
    endpoints was published at the budget. Withholding would have frozen its
    config indefinitely. Those clusters' endpoints were left out, so the client
    kept the ones it had.
  * `flip_release`: a route change held for a newly referenced cluster was
    published at the budget because the cluster never became ready. Routes to it
    fail until it does, and the route, listener and secret updates held with it
    resume.
  * `warm_release`: a warm client withheld for the whole warm withhold bound,
    because referenced clusters were missing from CDS, was published. Routes to
    those clusters fail until they appear.
  * `carry_expired`: a published client stopped being carried a referenced
    cluster that stayed missing from CDS for the warm withhold bound. Routes to it
    fail until it reappears.
  * `reference_ahead`: with `REFERENCED` xDS scoping, a hold kept only for
    the reference-ahead window (`KGW_XDS_SCOPING_REFERENCE_AHEAD`), which gives a
    newly published cluster time to reach the client before the configuration
    that names it, was released at the budget before the window closed. A hold
    whose window closes first ends as designed and is not counted.
* `flip_release`, `warm_release` and `carry_expired` mean some routes are
  failing; they are the modes to alert on.

### `kgateway_xds_snapshot_perclient_inconsistent_snapshots_total` Metric
* Recorded only when `KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK` is enabled, which is
  meant for test and CI environments. It has no labels.
* Counts per-client snapshots that, just before publication, carried an EDS or
  RDS resource nothing in the same snapshot references, or a listener naming a
  RouteConfiguration the snapshot lacks. go-control-plane withholds an ADS
  response that carries a resource the client did not request. The snapshot is
  still published.
* The publication paths keep snapshots consistent by construction, so any
  increment is a kgateway bug. Report it with the error logged alongside it,
  which names the client.

### `kgateway_xds_snapshot_perclient_reclaimed_total` Metric
* Counts per-client snapshot cache entries cleared after their client left and
  stayed gone for the grace period, which keeps controller memory from growing
  with proxy churn. It has no labels.
* Its rate follows how often proxies go away for good: scale-downs, rollouts and
  deleted gateways. A proxy that reconnects within the grace period keeps its
  entry and is not counted.

The settings behind these metrics are described in
[xDS publication and scoping settings](../../docs/guides/xds-publication-settings.md).
