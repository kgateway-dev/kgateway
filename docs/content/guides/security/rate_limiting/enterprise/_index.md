---
title: Server Config (Enterprise)
description: Advanced configuration for Gloo Edge Enterprise's rate-limit service.
weight: 40
---

To enforce rate limiting, Envoy queries an external rate limit server. You can configure changes to the rate limit server as follows.

* [Configure rate limiting query behavior](#query-behavior)
* [Change the default database](#database) from an internal Redis deployment to an external DynamoDB or Aerospike database
* [Debug the rate limit server](#debug)

## Configure Envoy rate limit query behavior {#query-behavior}

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

## Change the rate limit server's backing database {#database}

By default, the rate limit server is backed by a Redis instance that Gloo Edge deploys for you. Redis is a good choice for global rate limiting data storage because of its small latency. However, you might want to use a different database for the following reasons:
* Rate limiting across multiple data centers
* Replicating data for multiple replicas of the database
* Using an existing database
* Using a database that is external to the cluster, such as for data privacy concerns

Gloo Edge supports the following external databases for the rate limit server:
* [DynamoDB](#dynamodb)
* [Aerospike](#aerospike)

### DynamoDB-backed rate limit server {#dynamodb}

You can use DynamoDB as the backing storage database for the Gloo Edge rate limit server. DynamoDB is built for single-millisecond latencies. It includes features such as built-in replication ([DynamoDB Global Tables](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/GlobalTables.html)) that can help you set up global rate limiting across multiple instances or multiple data centers.

{{% notice note %}}
DynamoDB rate-limiting is a feature of **Gloo Edge Enterprise**, release 0.18.29+
{{% /notice %}}

1. Create a secret in your cluster that includes your AWS credentials for the DynamoDB that you want to use. For more information, see the [AWS docs](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/SettingUp.DynamoWebService.html).
   ```shell
   glooctl create secret aws
   ```
2. When you install Gloo Edge Enterprise version 0.18.29 or later, disable Redis and provide the rate limiting DynamoDB Helm chart configuration options instead.

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

## Debug the rate limit server {#debug}

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