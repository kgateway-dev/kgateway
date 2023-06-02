---
title: Passing through traffic for unsupported ciphers
weight: 50
description: Configure your gateway to pass through TLS traffic to an upstream clients for clients that present deprecated cipher suites. 
---

You can pass through traffic from clients that use cipher suites that are unsupported or deprecated in Envoy based on SNI (Server Name Indicator).

- **Cipher is supported**: If the cipher that the client presents is supported in Envoy, the TLS connection is terminated and the HTTP request is forwarded to the upstream. 
- **Cipher is not supported**: If the client presents an unsupported cipher, the TLS connection is not terminated. Instead, the request is passed through the gateway by using the TCP proxy in Envoy. The upstream server must be capable of performing the TLS termination.







Customer would like to support clients/servers that use TLS 1.2 ciphers that are not natively supported by Envoy. To achieve this, they would like to be able to configure (via Gloo Edge control plane APIs) lists of natively-supported and passthrough ciphers, which may differ by SNI domain. For incoming TLS sessions, if the client supports one of the "native" ciphers, then TLS should be terminated and the http requests processed according to Gloo Edge config. If the client supports one of the passthrough ciphers, then traffic will be routed to a passthrough server via TCP proxy.
