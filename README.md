

<h1 align="center">
    <img src="https://github.com/solo-io/gloo-mesh-enterprise/blob/main/docs/content/gateway/img/logo.svg" alt="Gloo Gateway v1 (formerly Gloo Edge)" width="600" height="155">
  <br>
  An Envoy-Powered API Gateway
</h1>

---

**IMPORTANT UPDATES:**</br>
Gloo Edge was renamed to Gloo Gateway to align with Solo's support for the Kubernetes Gateway API. The existing Gloo Edge v1 APIs were not changed and continue to be fully supported. To find out more about the new Gloo Gateway v2 API that is based on the Kubernetes Gateway API, see the v2 branch. 

---

Gloo Gateway is a feature-rich, Kubernetes-native ingress controller and next-generation API gateway. Gloo Gateway is exceptional in its function-level routing; its support for legacy apps, microservices and serverless; its discovery capabilities; its numerous features; and its tight integration with leading open-source projects. Gloo Gateway is uniquely designed to support hybrid applications in which multiple technologies, architectures, protocols, and clouds can coexist. 


[**Installation**](https://gloo.solo.io/installation/) &nbsp; |
&nbsp; [**Documentation**](https://gloo.solo.io) &nbsp; |
&nbsp; [**Blog**](https://www.solo.io/blog/?category=gloo) &nbsp; |
&nbsp; [**Slack**](https://slack.solo.io) &nbsp; |
&nbsp; [**Twitter**](https://twitter.com/soloio_inc) |
&nbsp; [**Enterprise Trial**](https://www.solo.io/products/gloo/#enterprise-trial)

<BR><center><img src="https://docs.solo.io/gloo-edge/main/img/gloo-architecture-envoys.png" alt="Gloo Gateway Architecture" width="906"></center>

## Summary

- [**Using Gloo Gateway**](#using-gloo-gateway)
- [**What makes Gloo Gateway unique**](#what-makes-gloo-gateway-unique)


## Using Gloo Gateway
- **Kubernetes ingress controller** : Gloo Gateway can function as a feature-rich ingress controller, built on top of the Envoy Proxy. 
- **Next-generation API gateway** : Gloo Gateway provides a long list of API gateway features including rate limiting, circuit breaking, retries, caching, transformation, service-mesh integration, security, external authentication and authorization. 
- **Hybrid apps** : Gloo Gateway creates applications that route to backends implemented as microservices, serverless functions and legacy apps. This feature can help users to -
   - A) Gradually migrate from their legacy code to microservices and serverless.
   - B) Add new functionalities using cloud-native technologies while maintaining their legacy codebase.
   - C) Allow different teams in an organization choose different architectures. 
       See [here](https://www.solo.io/hybrid-app) for more on the Hybrid App paradigm. 


## What makes Gloo Gateway unique
- **Function-level routing allows integration of legacy applications, microservices and serverless** : Gloo Gateway can route requests directly to _functions_. Request to Function can be a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS Function, etc.), an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.), or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes Gloo Gateway the only API gateway that supports hybrid apps as well as the only one that does not tie the user to a specific paradigm. 
- **Gloo Gateway incorporates vetted open-source projects to provide broad functionality** : Gloo Gateway support high-quality features by integrating with top open-source projects, including gRPC, GraphQL, OpenTracing, NATS and more. Gloo Gateway's architecture allows rapid integration of future popular open-source projects as they emerge. 
- **Full automated discovery lets users move fast** : Upon launch, Gloo Gateway creates a catalog of all available destinations and continuously maintains it up to date. This takes the responsibility for 'bookkeeping' away from the developers and guarantees that new feature become available as soon as they are ready. Gloo Gateway discovers across IaaS, PaaS and FaaS providers as well as Swagger, gRPC, and GraphQL. 
- **Gloo Gateway integrates intimately with the user's environment** : with Gloo Gateway, users are free to choose their favorite tools for scheduling (such as K8s, Nomad, OpenShift, etc), persistence (K8s, Consul, etcd, etc) and security (K8s, Vault). 


## Next Steps
- Join us on our Slack channel: [https://slack.solo.io/](https://slack.solo.io/)
- Follow us on Twitter: [https://twitter.com/soloio_inc](https://twitter.com/soloio_inc)
- Check out the docs: [https://gloo.solo.io](https://gloo.solo.io)
- Check out the code and contribute: [Contribution Guides](/devel/contributing)
- Contribute to the [Docs](docs/)

### Thanks

**Gloo Gateway** would not be possible without the valuable open-source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io).


# Security

*Reporting security issues* : We take Gloo Edge's security very seriously. If you've found a security issue or a potential security issue in Gloo Edge, please DO NOT file a public Github issue, instead send your report privately to [security@solo.io](mailto:security@solo.io).
