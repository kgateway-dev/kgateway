---
menuTitle: Network Encryption
title: Network Encryption
weight: 10
description: Configure Gloo Edge to use TLS upstream, downstream, and with Envoy
---

Network security and encryption is incredibly important, especially for public facing services or services that carry sensitive data. Gloo Edge can assist with the following use cases:

 * Perform *TLS termination* for downstream clients, unencrypting traffic arriving from downstream clients
 * Loading client certificates to perform mutual TLS with an *upstream server* which is already serving TLS
 * Configure mutual TLS with the Envoy proxy served by the xDS service on the Gloo Edge pod

The following guides provide more detail on how to configure each feature:

{{% children description="true" %}}

