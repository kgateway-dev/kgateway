---
title: Server Config (Enterprise)
description: Advanced configuration for Gloo Edge Enterprise's rate-limit service.
weight: 40
---

##### Configuring Envoy rate limit behavior

Envoy queries an external server (backed by redis by default) to achieve global rate limiting. You can set a timeout for the
query, and what to do in case the query fails. By default, the timeout is set to 100ms, and the failure policy is
to allow the request.

To change the timeout to 200ms, use the following command:

```bash
glooctl edit settings --name default --namespace gloo-system ratelimit --request-timeout=200ms
```

To deny requests when there's an error querying the rate limit service, use this command:

```bash
glooctl edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true
```

##### Debugging

You can check if envoy has errors with rate limiting by examining its stats that end in `ratelimit.error`.
`glooctl proxy stats` displays the stats from one of the envoys in your cluster.

You can introspect the rate limit server to see the configuration that is present on the server. 
First, run this command to port-forward the server (assuming Gloo Edge Enterprise is installed to the `gloo-system` namespace): 
`kubectl port-forward -n gloo-system deploy/rate-limit 9091`.

Now, navigate to `localhost:9091/rlconfig` to see the active configuration, or `localhost:9091` to see all the administrative
options. 

By default, the rate limit server uses redis as an in-memory cache of the current rate limit counters with their associated 
timeouts. To see the current value of rate limit counters, you can inspect redis. First, run 
`kubectl port-forward -n gloo-system deploy/redis 6379`. Then, invoke a tool like [redis_cli](https://redis.io/topics/rediscli)
to connect to the instance. `scan 0` is a useful query to see all the current counters, and `get COUNTER` can be used 
to inspect the current value.  

##### DynamoDB-backed Rate Limit Service
By default, Gloo Edge's built-in rate-limit service is backed by Redis. Redis is a good choice for a global rate-limit data
store because of its small latency. Unfortunately, it can fall short in cases when users desire cross data center
rate-limiting, as Redis doesn't support replication or multi-master configurations.

DynamoDB can pickup the slack here by leveraging its built-in replication 
([DynamoDB Global Tables](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/GlobalTables.html)). DynamoDB
is built for single-millisecond latencies, so you can trade some performance in exchange for truly global rate limiting.

{{% notice note %}}
DynamoDB rate-limiting is a feature of **Gloo Edge Enterprise**, release 0.18.29+
{{% /notice %}}

To enable DynamoDB rate-limiting (disables Redis), install Gloo Edge with helm and provide an override for 
`rateLimit.deployment.dynamodb.secretName`. This secret can be generated using `glooctl create secret aws`.

Once deployed, the rate limit service will create the rate limits DynamoDB table (default `rate-limits`) in the
provided aws region using the provided creds. If you want to turn the table into a globally replicated table, you
will need to select which regions to replicate to in the DynamoDB aws console UI.

The full set of DynamoDB related config follows:

| option                                                    | type     | description                                                                                                                                                                                                                                                    |
| --------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| rateLimit.deployment.dynamodb.secretName                  | string   | Required: name of the aws secret in gloo's installation namespace that has aws creds |
| rateLimit.deployment.dynamodb.region                      | string   | aws region to run DynamoDB requests in (default `us-east-2`) |
| rateLimit.deployment.dynamodb.tableName                   | string   | DynamoDB table name used to back rate limit service (default `rate-limits`) |
| rateLimit.deployment.dynamodb.consistentReads             | bool     | if true, reads from DynamoDB will be strongly consistent (default `false`) |
| rateLimit.deployment.dynamodb.batchSize                   | uint8    | batch size for get requests to DynamoDB (max `100`, default `100`) |
