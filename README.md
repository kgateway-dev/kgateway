

<h1 align="center">
    <img src="https://gloo.solo.io/img/Gloo-01.png" alt="Gloo" width="200" height="242">
  <br>
  An Envoy-Powered API Gateway
</h1>

Gloo is a feature-rich, Kubernetes-native ingress controller, and next-generation API gateway. Gloo is exceptional in its function-level routing; its support for legacy apps, microservices and serverless; its discovery capabilities; its numerous features; and its tight integration with leading open-source projects. Gloo is uniquely designed to support hybrid applications, in which multiple technologies, architectures, protocols, and clouds can coexist. 


[**Installation**](https://gloo.solo.io/installation/) &nbsp; |
&nbsp; [**Documentation**](https://gloo.solo.io) &nbsp; |
&nbsp; [**Blog**](https://medium.com/solo-io/announcing-gloo-the-function-gateway-3f0860ef6600) &nbsp; |
&nbsp; [**Slack**](https://slack.solo.io) &nbsp; |
&nbsp; [**Twitter**](https://twitter.com/soloio_inc)
&nbsp; [**Enterprise Trial**](https://www.solo.io/glooe)

<BR><center><img src="https://gloo.solo.io/introduction/gloo_diagram.png" alt="Gloo Architecture" width="906"></center>

## Summary

- [**Using Gloo**](#using-gloo)
- [**What makes Gloo unique**](#what-makes-gloo-unique)


## Using Gloo
- **Kubernetes ingress controller**: Gloo can function as a feature-rich ingress controller, built on top of the Envoy Proxy. 
- **Next-generation API gateway** : Gloo provides a long list of API gateway features, including rate limiting, circuit breaking, retries, caching, external authentication and authorization, transformation, service-mesh integration, and security. 
- **Hybrid apps**: Gloo creates applications that route to backends implemented as microservices, serverless functions, and legacy apps. This feature can help users to gradually migrate from their legacy code to microservices and serverless; can let users add new functionalities using cloud-native technologies while maintaining their legacy codebase; can be used in cases where different teams in an organization choose different architectures; and more. See [here](https://www.solo.io/hybrid-app) for more on the Hybrid App paradigm. 


## What makes Gloo unique
- **Function-level routing allows integration of legacy applications, microservices and serverless**: Gloo can route requests directly to _functions_, which can be a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS function, etc.), an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.), or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes Gloo the only API gateway that supports hybrid apps, as well as the only one that does not tie the user to a specific paradigm. 
- **Gloo incorporates vetted open-source projects to provide broad functionality**: Gloo support high-quality features by integrating with top open-source projects, including gRPC, GraphQL, OpenTracing, NATS and more. Gloo's architecture allows rapid integration of future popular open-source projects as they emerge. 
- **Full automated discovery lets users move fast**: Upon launch, Gloo creates a catalog of all available destinations, and continuously maintains it up to date. This takes the responsibility for 'bookkeeping' away from the developers, and guarantees that new feature become available as soon as they are ready. Gloo discovers across IaaS, PaaS and FaaS providers, as well as Swagger, gRPC, and GraphQL. 
- **Gloo integrates intimately with the user's environment**: with Gloo, users are free to choose their favorite tools for scheduling (such as K8s, Nomad, OpenShift, etc), persistence (K8s, Consul, etcd, etc) and security (K8s, Vault). 


## Next Steps
- Join us on our Slack channel: [https://slack.solo.io/](https://slack.solo.io/)
- Follow us on Twitter: [https://twitter.com/soloio_inc](https://twitter.com/soloio_inc)
- Check out the docs: [https://gloo.solo.io](https://gloo.solo.io)
- Check out the code and contribute: [Contribution Guide](CONTRIBUTING.md)
- Contribute to the [Docs](https://github.com/solo-io/solo-docs)

### Thanks

**Gloo** would not be possible without the valuable open-source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io).


# Security

*Reporting security issues:* We take Gloo's security very seriously. If you've found a security issue or a potential security issue in Gloo, please DO NOT file a public Github issue, instead send your report privately to [security@solo.io](mailto:security@solo.io).