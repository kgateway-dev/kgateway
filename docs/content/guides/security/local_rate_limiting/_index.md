---
title: Local rate limiting
weight: 32
---

Limit the number of requests to your gateway or upstream services before the requests reach the rate limiting server in your cluster by using the Envoy local rate limiting filter. For any subsequent requests that are sent after the limit is reached, a 429 (Too Many Requests) HTTP response code is returned to the client. 

Refer to the following topics to learn more about local rate limiting. 

{{% children %}}