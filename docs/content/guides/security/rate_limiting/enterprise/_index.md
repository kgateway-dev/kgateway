---
title: Server Config (Enterprise)
description: Advanced configuration for Gloo Edge Enterprise's rate-limit service.
weight: 40
---

To enforce rate limiting, Envoy queries an external rate limit server. You can configure changes to the rate limit server as follows.

* [Configure rate limiting query behavior](#query-behavior)
* [Change the default database](#database) from an internal Redis deployment to an external Aerospike, DynamoDB, or Redis database
* [Debug the rate limit server](#debug)

## Configure Envoy rate limit query behavior {#query-behavior}

To achieve global rate limiting, Envoy queries an external server that is backed by Redis by default. You can configure the default query behavior as described in the following table.

| Query behavior | Default value | How you can change the default | 
| --- | --- | --- |
| Timeout for the query | 100ms | Change the timeout duration, such as to 200ms.<br>`glooctl edit settings --name default --namespace gloo-system ratelimit --request-timeout=200ms` |
| Query failure behavior | Allow the request | Deny the request if the rate limit service cannot complete the query.<br>`glooctl edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true`|


## Change the rate limit server's backing database {#database}

By default, the rate limit server is backed by a Redis instance that Gloo Edge deploys for you. Redis is a good choice for global rate limiting data storage because of its low latency. However, you might want to use a different database for the following reasons:
* Rate limiting across multiple data centers
* Replicating data for multiple replicas of the database
* Using an existing database
* Using a database that is external to the cluster, such as for data privacy concerns

Gloo Edge supports the following external databases for the rate limit server:
* [Aerospike](#aerospike)
* [DynamoDB](#dynamodb)
* [Redis](#redis)

### Aerospike-backed rate limit server {#aerospike}

You can use [Aerospike](https://docs.aerospike.com/database) as the backing storage database for the Gloo Edge rate limit server. Aerospike is a real-time data platform with support for helpful features such as in-memory storage and streaming.

{{% notice note %}}
You can use Aerospike with **Gloo Edge Enterprise** version 1.13.0 or later.<br><br>
If you use also use Aerospike to store your Gloo Portal API keys, your Aerospike configurations must match. For example, use the same Aerospike IP address, port, and namespace in your Gloo Portal Storage custom resoure configuration and the rate limit server.
{{% /notice %}}

1. Create an Aerospike database instance to use as the backing storage for the rate limit server. For setup steps, see the [Gloo Portal documentation](https://docs.solo.io/gloo-portal/main/guides/portal_features/apikey_storage/). 
2. To rate limit APIs that you manage with Gloo Portal, make sure that your configuration matches the configuration that you used with your [Gloo Portal Storage custom resource](https://docs.solo.io/gloo-portal/main/guides/portal_features/apikey_storage/).
3. [Install]({{< versioned_link_path fromRoot="/installation/enterprise/">}}) or [upgrade]({{< versioned_link_path fromRoot="/operations/upgrading/">}}) your Gloo Edge Enterprise Helm installation by completing the following steps:
   1. Disable the default Redis server backing storage by setting `rateLimit.enabled` to `false`.
   2. Provide the rate limiting Aerospike Helm chart configuration options, as shown in the following table. These values match what you configured in your Aerospike database setup. 

| Option | Type | Description |
| --- | --- | --- |
|rateLimit.deployment.aerospike.address|string|The IP address or hostname of the Aerospike database. The address must be reachable from Gloo Edge, such as in a virtual machine with a public IP address or in a pod in the cluster. By setting this value, you also enable Aerospike database as the backing storage for the rate limit service.|
|rateLimit.deployment.aerospike.namespace|string|The Aerospike namespace of the database. The default value is `solo-namespace`.|
|rateLimit.deployment.aerospike.set|string|The Aerospike name of the database set. The default value is `ratelimiter`.|
|rateLimit.deployment.aerospike.port|int|The port of the `rateLimit.deployment.aerospike.address`. The default port is `3000`.|
|rateLimit.deployment.aerospike.batchSize|int|The size of the batch, which is the number of keys sent in the request. The default value is `5000`.|
|rateLimit.deployment.aerospike.commitLevel|int|The level of guaranteed consistency for transaction commits on the Aerospike server. For possible values, see the [Aerospike commit policy](https://github.com/aerospike/aerospike-client-go/blob/master/commit_policy.go). The default value is `1`.|
|rateLimit.deployment.aerospike.readModeSC|int|The read mode for strong consistency (SC) options. For possible values, see the [Aerospike read mode SC](https://github.com/aerospike/aerospike-client-go/blob/master/read_mode_sc.go). The default value is `0`.|
|rateLimit.deployment.aerospike.readModeAP|int|The read mode for availability (AP). For possible values, see the [Aerospike read mode AP](https://github.com/aerospike/aerospike-client-go/blob/master/read_mode_ap.go). The default value is `0`.|
|rateLimit.deployment.aerospike.tls.name|string|The subject name of the TLS authority. For more information, see the [Aerospike docs](https://docs.aerospike.com/reference/configuration#tls-name).|
|rateLimit.deployment.aerospike.tls.version|string|The TLS version. Versions 1.0, 1.1, 1.2, and 1.3 are supported. The default value is `1.3`.|
|rateLimit.deployment.aerospike.tls.insecure|bool|The TLS insecure setting. If set to `true`, the authority of the certificate on the client's end is not authenticated. You might use insecure mode in non-production environments when the certificate is not known. The default value is `false`.|
|rateLimit.deployment.aerospike.tls.certSecretName|string| The name of the `kubernetes.io/tls` secret that has the `tls.crt` and `tls.key` data.|
|rateLimit.deployment.aerospike.tls.rootCASecretName|string|The secret name for the Opaque root CA that sets the key as `tls.crt`.|
|rateLimit.deployment.aerospike.tls.curveGroups[]|string|The TLS identifier for an elliptic curve. For more information, see [TLS supported groups](https://www.iana.org/assignments/tls-parameters/tls-parameters.xml#tls-parameters-8).|

### DynamoDB-backed rate limit server {#dynamodb}

You can use DynamoDB as the backing storage database for the Gloo Edge rate limit server. DynamoDB is built for single-millisecond latencies. It includes features such as built-in replication ([DynamoDB Global Tables](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/GlobalTables.html)) that can help you set up global rate limiting across multiple instances or multiple data centers.

{{% notice note %}}
You can use DynamoDB with **Gloo Edge Enterprise** version 0.18.29 or later.
{{% /notice %}}

1. [Create a secret]({{< versioned_link_path fromRoot="/reference/cli/glooctl_create_secret_aws/">}}) in your cluster that includes your AWS credentials for the DynamoDB that you want to use. The secret must be in the same namespace as your Gloo installation, such as `gloo-system`. For more information, see the [AWS docs](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/SettingUp.DynamoWebService.html).
   ```shell
   glooctl create secret aws -n gloo-system --access-key <aws_access_key> --secret-key <aws_secret_key>
   ```
2. [Install]({{< versioned_link_path fromRoot="/installation/enterprise/">}}) or [upgrade]({{< versioned_link_path fromRoot="/operations/upgrading/">}}) your Gloo Edge Enterprise Helm installation by completing the following steps:
   1. Disable the default Redis server backing storage by setting `rateLimit.enabled` to `false`.
   2. Provide the rate limiting DynamoDB Helm chart configuration options, as shown in the following table.

| Option | Type | Description |
| --- | --- | --- |
| rateLimit.deployment.dynamodb.secretName                  | string   | Required: The name of the secret with the AWS credentials that you previously created. The secret must be in the same namespace as your Gloo installation, such as `gloo-system`.|
| rateLimit.deployment.dynamodb.region                      | string   | The AWS region to run the DynamoDB requests in. The default region is `us-east-2`. |
| rateLimit.deployment.dynamodb.tableName                   | string   | The name of the DynamoDB table that backs the rate limit service. The default name is `rate-limits`. |
| rateLimit.deployment.dynamodb.consistentReads             | bool     | If `true`, the reading response from DynamoDB is _strongly consistent_, or the most up-to-date data. The default value is `false`, or _eventually consistent_, which might be less accurate but also has lower latency and less chance of a 500 response than _strongly consistent_. For more information, see the [DynamoDB docs](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.ReadConsistency.html).|
| rateLimit.deployment.dynamodb.batchSize                   | uint8    | The batch size for `GET` requests to DynamoDB. The max value is `100`, which is also the default value. |

As part of the rate limit service deployment, Gloo Edge uses the provided AWS credentials to automatically create the rate limits DynamoDB table (default name `rate-limits`) in your AWS region (default `us-east-2`). If you want to turn the table into a globally replicated table, you
can select the regions to replicate to in the DynamoDB AWS console UI.

### Redis-backed rate limit server {#redis}

You can use a clustered Redis instance as the backing storage database for the Gloo Edge rate limit server. Redis is an open source, in-memory database with features such as data persistence, server-side scripting and Redis Functions, extensibility, and sharding for horizontal scalability, clustering for high availability. For more information, see [the Redis docs](https://redis.io/docs/getting-started/).

1. Install a clustered Redis instance in your cluster. The following example uses a Helm chart and Redis version 6. You can use the following versions of Redis with Gloo Edge.
   * Redis 6
   * Redis 7 in Gloo Edge 1.13 or later
   ```sh
   helm repo add bitnami https://charts.bitnami.com/bitnami
   helm repo update
   helm install redis -n redis bitnami/redis-cluster --set fullnameOverride=redis --set global.redis.password=redis --set image.tag=6.2.12-debian-11-r0 --version 8.1.1 --create-namespace
   ```
2. Create a secret with your Redis credentials in the same namespace as your Gloo Edge installation, such as `gloo-system`. Make sure to encode your credentials in base64. For more information, see the [Redis security docs](https://redis.io/docs/management/security/acl/).
   ```yaml
   cat << EOF | kubectl apply -f -
   apiVersion: v1
   kind: Secret
   metadata:
     name: redis
     namespace: gloo-system
     labels:
       app.kubernetes.io/instance: redis
   type: Opaque
   data:
     redis-password: <base64_password>
     users.acl: <base64_access-control-list>
   EOF
   ```
3. [Install]({{< versioned_link_path fromRoot="/installation/enterprise/">}}) or [upgrade]({{< versioned_link_path fromRoot="/operations/upgrading/">}}) your Gloo Edge Enterprise Helm installation by completing the following steps:
   1. Disable the default Redis server backing storage by setting `rateLimit.enabled` to `false`.
   2. Provide the rate limiting Redis Helm chart configuration options, as shown in the following table.

      ```yaml
      redis:
        disabled: true
        clustered: true
        service:
          name: "redis.redis.svc"
          port: 6379
      ```

      | Option | Type | Description |
      | --- | --- | --- |
      | redis.disabled | bool | Set to `true` to disable a default Redis instance, so that you can bring your own clustered Redis. |
      | redis.clustered | bool | Set to `true` to bring your own clustered Redis instance. |
      | redis.service.name | string | The name of the service to use to access the Redis instance, such as the default `redis.redis.svc`. |
      | redis.service.port | string | The port to access Redis on, which by default is `6379`.|

   3. Make sure that the rate limit server refers to the secret that you created. You can choose between the following options.
      * Override the rate limit deployment Helm values to refer to the secret. For values, see the [Helm reference docs]({{< versioned_link_path fromRoot="/reference/helm_chart_values/">}}).
      * Restart the rate limit pods after creating the secret and re-installing with the clustered Redis instance.

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
