---
title: Server Config (Enterprise)
description: Advanced configuration for Gloo Enterprise's rate-limit service.
weight: 20
---

##### DynamoDB-backed Rate Limit Service
By default, Gloo's built-in rate-limit service is backed by Redis. Redis is a good choice for a global rate-limit data
store because of its small latency. Unfortunately, it can fall short in cases when users desire cross data center
rate-limiting, as Redis doesn't support replication or multi-master configurations.

DynamoDB can pickup the slack here by leveraging its built-in replication 
([DynamoDB Global Tables](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/GlobalTables.html)). DynamoDB
is built for single-millisecond latencies, so you can trade some performance in exchange for truly global rate limiting.

{{% notice note %}}
DynamoDB rate-limiting is a feature of **Gloo Enterprise**, release 0.18.29+
{{% /notice %}}

To enable DynamoDB rate-limiting (disables Redis), install Gloo with helm and provide an override for 
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
