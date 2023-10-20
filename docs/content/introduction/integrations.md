---
title: Integrations
weight: 50
---

Gloo Edge has integrations with several other applications and services to either enhance the functionality of those services or add functionality to Gloo Edge. Gloo Edge currently offers the following integrations:

* **[Kubernetes Ingress](#kubernetes-ingress)** - Use Gloo Edge as the ingress controller on your Kubernetes cluster
* **[AWS Elastic Load Balancers](#aws-elastic-load-balancers)** - Augment the AWS Elastic Load Balancer with Gloo Edge
* **[Service Mesh](#service-mesh)** - Provide edge ingress for a service mesh deployment like Istio, Linkerd, or AWS App Mesh
* **[Let's Encrypt](#lets-encrypt)** - Provision and manage certificates for Gloo Edge using Let's Encrypt and cert-manager

---

## Kubernetes Ingress

Kubernetes ingress controllers provide simple traffic routing in a Kubernetes cluster. The ingress resource can provide an externally reachable URL, load balancing services, or terminate TLS traffic. Gloo Edge can be installed in ingress mode to provide ingress controller services to a Kubernetes cluster. The ingress specification provides relatively basic features and functionality. For more robust control over traffic on your Kubernetes cluster, we recommend deploying Gloo Edge in gateway mode.

---

## AWS Elastic Load Balancers

AWS provides three (3) types of load balancers: Classic Load Balancer (ELB or CLB), Network Load Balancer (NLB), and an Application Load Balancer (ALB). Gloo Edge works well with any of these AWS load balancers, though our recommendation is to prefer the AWS Network Load Balancer as that has the least capabilities overlap and the best value when paired with Gloo Edge. Gloo Edge provides all of the L7 HTTP/S and gRPC routing, security, and web application firewall capabilities (and much more) that either Classic Load Balancer or Application Load Balancer provides. And since Gloo Edge leverages Envoy, Gloo Edge benefits from the growing Envoy community, including AWS contributions, and Envoy’s extensible filter architecture to provide for customized extensions when needed.

There are many benefits to pairing Gloo Edge with one of AWS Elastic Load Balancers (ELB). The ELB has a better understanding of the AWS infrastructure, and so it can provide cross availability zone failover and integration with AWS services like AWS Certificate Manager, AWS CLI & CloudFormation, and Route 53 (DNS). Gloo Edge provides developer access to traffic routing without needing to grant developers access to AWS resources. An NLB can handle the front-end traffic ingress, and then additional routing decisions can be made by Gloo Edge. Service routing logic can be stored with application code and run through a CI/CD pipeline for validation.

Our [AWS Elastic Load Balancer guide]({{% versioned_link_path fromRoot="/guides/integrations/aws/" %}}) will walk you through combining an AWS Network Load Balancer with Gloo Edge and AWS Elastic Kubernetes Service.

---

## Service Mesh

Service mesh technologies solve problems with service-to-service communications across cloud networks. Problems such as service identity, consistent L7 network telemetry gathering, service resilience, traffic routing between services, as well as policy enforcement (like quotas, rate limiting, etc) can be solved with a service mesh. 

For a service mesh to operate correctly, it needs a way to get traffic into the mesh. The problems with getting traffic from the edge into the cluster are a bit different from service-to-service problems. Things like edge caching, first-hop security and traffic management, OAuth and end-user authentication/authorization, per-user rate limiting, web-application firewalling, etc are all things an ingress gateway can and should help with. Gloo Edge solves these problems and complements any service mesh including Istio, Linkerd, Consul Connect, and AWS App Mesh.

Checkout our [service mesh guides section]({{% versioned_link_path fromRoot="/guides/integrations/service_mesh/" %}}) for more information about deploying Gloo Edge with any of these service mesh technologies.

---

## Let's Encrypt {#lets-encrypt}

Supporting HTTP traffic over TLS is almost a given for web services. The procurement and installation of certificates to support TLS can be a hassle. Let's Encrypt and cert-manager offer a simple and automated approach to certificate management. [Cert-manager](https://cert-manager.io/docs/) is a native Kubernetes certificate management controller that can assist with issuing certificates from sources like Let's Encrypt and HashiCorp Vault. [Let's Encrypt](https://letsencrypt.org) is a free, automated, open certificate authority providing digital certificates to enable TLS for a website.

Gloo Edge integrates with cert-manager and Let's Encrypt to automate the procurement of TLS certificates for services offered through the Gateway Proxy. [Our guide]({{% versioned_link_path fromRoot="/guides/integrations/cert_manager/" %}}) walks you through the process of integrating Gloo Edge with Let's Encrypt and cert-manager, using Amazon Route 53 to provide the DNS services for certificate validation.

---

## Next Steps

Now that you have an understanding of the integrations supported by Gloo Edge, we have a few suggested paths:

* **[Traffic Management]({{% versioned_link_path fromRoot="/introduction/traffic_management/" %}})** - learn more about Gloo Edge and traffic management
* **[Setup]({{% versioned_link_path fromRoot="/installation/" %}})** - Deploy your own instance of Gloo Edge
* **[Integration guides]({{% versioned_link_path fromRoot="/guides/integrations/" %}})** - Set up one of the integrations described above