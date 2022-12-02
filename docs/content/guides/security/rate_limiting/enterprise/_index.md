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

To achieve global rate limiting, Envoy queries an external server that is backed by Redis by default. You can configure the default query behavior as described in the following table.

| Query behavior | Default value | How you can change the default | 
| --- | --- | --- |
| Timeout for the query | 100ms | Change the timeout duration, such as to 200ms.<br><`glooctl edit settings --name default --namespace gloo-system ratelimit --request-timeout=200ms` |
| What to do in case of query failure | Allow the request | Deny the request if the rate limit service cannot complete the query.<br>`glooctl edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true`|


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
You can use DynamoDB with **Gloo Edge Enterprise** version 0.18.29 or later.
{{% /notice %}}

1. Create a secret in your cluster that includes your AWS credentials for the DynamoDB that you want to use. The secret must be in the same namespace as your Gloo installation, such as `gloo-system`. For more information, see the [AWS docs](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/SettingUp.DynamoWebService.html).
   ```shell
   glooctl create secret aws -n gloo-system
   ```
2. When you [install]({{< versioned_link_path fromRoot="/installation/enterprise/">}}) or [upgrade]({{< versioned_link_path fromRoot="/operations/upgrading/">}}) your Gloo Edge Enterprise Helm installation, complete the following steps:
   1. Disable the default Redis server backing storage.
   2. Provide the rate limiting DynamoDB Helm chart configuration options, as shown in the following table.

| Option | Type | Description |
| --- | --- | --- |
| rateLimit.deployment.dynamodb.secretName                  | string   | Required: The name of the secret with the AWS credentials that you previously created. The secret must be in the same namespace as your Gloo installation, such as `gloo-system`.|
| rateLimit.deployment.dynamodb.region                      | string   | The AWS region to run the DynamoDB requests in. The default region is `us-east-2`. |
| rateLimit.deployment.dynamodb.tableName                   | string   | The name of the DynamoDB table that backs the rate limit service. The default name is `rate-limits`. |
| rateLimit.deployment.dynamodb.consistentReads             | bool     | If `true`, the reading response from DynamoDB is _strongly consistent_, or the most up to date data. The default value is `false`, or _eventually consistent_, which might be less accurate but also has lower latency and less chance of a 500 response than _strongly consistent_. For more information, see the [DynamoDB docs](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.ReadConsistency.html).|
| rateLimit.deployment.dynamodb.batchSize                   | uint8    | The batch size for `GET` requests to DynamoDB. The max value is `100`, which is also the default value. |

As part of the rate limit service deployment, Gloo Edge uses the provided AWS credentials to automatically create the rate limits DynamoDB table (default name `rate-limits`) in your AWS region (default `us-east-2`). If you want to turn the table into a globally replicated table, you
can select the regions to replicate to in the DynamoDB AWS console UI.

## Debug the rate limit server {#debug}

1. Show the states from the Envoy proxy in your cluster.
   ```shell
   glooctl proxy stats
   ```
2. In the output, check for rate limiting errors from stats that end in `ratelimit.error`.
3. Start the rate limiting server locally, such as with the following command.
   ```shell
   kubectl port-forward -n gloo-system deploy/rate-limit 9091
   ```
4. Check the active configuration of the rate limiting server by opening the following localhost URL.
   ```shell
   localhost:9091/rlconfig
   ```

   For all administrative options, open the following localhost URL.
   ```shell
   localhost:9091
   ```
5. Check the backing storage database of the rate limit server.
   * **Default Redis server**: By default, the rate limit server uses Redis as an in-memory cache of the current rate limit counters with their associated timeouts. To check the current value of rate limit counters, inspect Redis.
     1. Start the Redis instance locally, such as with the following command.
        ```shell
        kubectl port-forward -n gloo-system deploy/redis 6379
        ```
     2. Connect to the Redis instance, such as with the [redis_cli](https://redis.io/topics/rediscli) tool. 
     3. Query the Redis instance, such as the following.
        * `scan 0` to list all the current counters.
        * `get COUNTER` to inspect the current value of a counter.
   * **DynamoDB**: Refer to the DynamoDB docs, such as [Query data](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/getting-started-step-5.html) or [Common Errors](https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/CommonErrors.html).
   * **Aerospike**: Refer to the Aerospike docs, such as [Troubleshooting](https://docs.aerospike.com/server/operations/troubleshoot).
