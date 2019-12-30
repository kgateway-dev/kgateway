---
title: Stats and Admin Ports
weight: 5
description: Useful ports to know for viewing stats and admin config
---

## Envoy Admin
Envoy's admin port is `19000` by default.
```bash
kubectl port-forward deployment/gateway-proxy 19000
```
```
Forwarding from 127.0.0.1:19000 -> 19000
Forwarding from [::1]:19000 -> 19000
```

More information on the large amount of features available in this admin view can be found in the [envoy docs](https://www.envoyproxy.io/docs/envoy/v1.7.0/operations/admin).

## Gloo Admin
If the `START_STATS_SERVER` environment variable is set to `true` in Gloo's pods, they will listen on port `9091`. Functionality available on that port includes Prometheus metrics at `/metrics` (see more on Gloo metrics [here](../metrics)), as well as enables admin functionality like changing the logging levels and getting a stack dump. 
