---
title: Debug Logging and Stats
weight: 5
description: Useful patterns to know for debug logging, viewing stats and admin config
---

## Envoy Administration Interface
Envoy's admin port is `19000` by default.
```bash
kubectl port-forward -n gloo-system deployment/gateway-proxy 19000
```
```
Forwarding from 127.0.0.1:19000 -> 19000
Forwarding from [::1]:19000 -> 19000
```

To enable debug logging, run:
```
curl "localhost:19000/logging?level=debug" -XPOST
```

To view the logs, go to your terminal and run:
```
kubectl logs -n gloo-system deploy/gateway-proxy -f
```

To view the stats, go to [localhost:19000/stats](http://localhost:19000/stats). Go to [localhost:19000](http://localhost:19000)
for additional information.

More information on the large amount of features available in this admin view can be found in the [envoy docs](https://www.envoyproxy.io/docs/envoy/v1.7.0/operations/admin).

## Gloo Administration Interface
If the `START_STATS_SERVER` environment variable is set to `true` in Gloo's pods, they will listen on port `9091`. Functionality available on that port includes Prometheus metrics at `/metrics` (see more on Gloo metrics [here]({{% versioned_link_path fromRoot="/observability/metrics/" %}}), as well as enables admin functionality like getting a stack dump.

For example, to enable debug logging on the gloo pod, run: 
```
kubectl port-forward -n gloo-system deployment/gloo 9091
```
Then go to [localhost:9091](http://localhost:9091) on your computer, and click to change the log level on that page.

To view the logs, run:
```
kubectl logs -n gloo-system deploy/gloo -f
```

## Feature Logging