---
title: Metrics (Enterprise)
weight: 50
description: Use prometheus metrics to determine when requests are near or over rate-limits.
---

In this tutorial we will see how to use the rate-limit service's default prometheus metrics to alert when rules are
near breach, and alert once broken.

{{% notice note %}}
For **Gloo Edge Enterprise**, you will need to enable rate-limit metrics using the following helm values:
```yaml
rateLimit:
  deployment:
    stats:
      enabled: true
```
Prior versions of Gloo Edge Enterprise published rate-limit metrics to port 16070, which isn't scraped by the default
prometheus installation included with Gloo Edge Enterprise.
{{% /notice %}}

### Reference

The available rate-limit metrics are:

|Metric|Description|Aggregation|Tag|
|------|-----------|-----------|---|
|`ratelimit.solo.io/over_limit`|The number of hits over the limit|Sum|`descriptor`|
|`ratelimit.solo.io/near_limit`|The number of hits near the limit (near limit is > 80% allowed by rule)|Sum|`descriptor`|
|`ratelimit.solo.io/total_hits`|The total number of hits|Sum|`descriptor`|
|`ratelimit.solo.io/descriptor_miss`|The number times a descriptor was ignored as it did not match the configuration|Count|N/A|
|`ratelimit.solo.io/errors`|The number of errors encountered by the rate-limit server|Count|`type`|

The `descriptor` tag (i.e., a prometheus [label](https://prometheus.io/docs/practices/naming/#labels)) corresponds to
the rule being referenced.

In this way, you can monitor and alert based on rate-limit rules/breaches, and quickly determine which rule was
triggered (and by how much).

### Example

The rest of this page is dedicated to a demonstration of some of Prometheus' and Grafana's basic features, in tandem
with the exposed rate-limit metrics. If you are already familiar with Prometheus and Grafana, you won't get much out
of this tutorial.

### Setup
Let's start by setting up the virtual service and settings resources to match the rule priority
[example]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/#rule-priority-and-weights" %}}).

### Triggering limits

Run the following; you should get HTTP 429 Too Many Requests on the second request.
```shell
curl -H "x-type: Whatsapp" -H "x-number: 311" --head $(glooctl proxy url)
curl -H "x-type: Whatsapp" -H "x-number: 311" --head $(glooctl proxy url)
```

Now let's take a look at the published metrics.

Port-forward the Gloo Edge Enterprise's Prometheus installation to port 9090:

```shell
kubectl port-forward -n gloo-system deployment/glooe-prometheus-server 9090
```

And open the Prometheus UI at [localhost:9090](http://localhost:9090).

In the dropdown, you can select the rate-limit metric you'd like to investigate. Let's investigate
`ratelimit_solo_io_over_limit`.

Add a PromQL query (prometheus query language) for `ratelimit_solo_io_over_limit`, choose the appropriate time window,
and execute. If you click the graph tab, you should see a spike in queries over limit (click to enlarge):

![Prometheus Graph]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/metrics/PrometheusGraph.png" %}})

Note that the metric is labeled with `descriptor`: `customtype_Whatsapp`. This is saying that our request was over the
limit associated with the custom descriptor `Whatsapp` (Rule 2, limit all Whatsapp requests to 1/min). Note that I
actually made 5 requests over the limit -- if you followed this tutorial exactly you should see 1 request.

### Alerting

The prometheus [alerting docs](https://prometheus.io/docs/alerting/overview/) can be leveraged for customized setups.

Alternatively, we can take any of our PromQL queries (e.g., `ratelimit_solo_io_over_limit`) and create an alert for
this in Gloo Edge's Grafana installation.

Port-forward the Gloo Edge Enterprise's Grafana installation to port 3000:
```shell
kubectl port-forward deployment/glooe-grafana 3000 -n gloo-system
```
And open the Prometheus UI at [localhost:3000](http://localhost:3000).

Enter the Grafana credentials (by default, `admin/admin`).

On the left side there is a bell, hover on it and select alerting -> notification channels. This should show the
following:

![Create Notification Channel]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/metrics/CreateNotificationChannel.png" %}})

For the sake of example, choose email, and save. We have not configured SMTP for Grafana, so the alerts won't actually
get delivered.

Now let's create a dashboard and alert for the metric you'd like to alert on.

Go to the main menu and select "create a dashboard". Fill in the PromQL query and as follows:

![Create Dashboard]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/metrics/CreateDashboard.png" %}})

In the alert page, you should see the email destination you created as a valid destination. Select it, and save your
dashboard:

![Alert Setup]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/metrics/AlertSetup.png" %}})

### Further Drill Down

Sometimes you'd like to further drill down into the offending requests, as well as requests that contributed to
rate-limit rule breaches. If the default metrics aren't enough, the next recommended course of action is to do some
analysis of your access logs. To get started, you can check out our access logs example
[here]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/access_logs" %}}).
