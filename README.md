

<h1 align="center">
    <img src="https://github.com/solo-io/gloo/blob/main/docs/content/img/logo.svg" alt="Gloo Gateway" width="800">
  <br> 
  An Envoy-Powered API Gateway
</h1>

## Important Update

> **Important**
> Gloo Gateway is now a fully conformant Kubernetes Gateway API implementation!
>
> The existing Gloo Edge APIs were not changed and continue to be fully supported.

## About Gloo Gateway
Gloo Gateway is a feature-rich, fast, and flexible Kubernetes-native ingress controller and next-generation API gateway that is built on top of [Envoy proxy](https://www.envoyproxy.io). It excels in function-level routing, supports legacy apps, microservices and serverless, offers robust discovery capabilities, integrates seamlessly with open-source projects, and is designed to support hybrid applications with various technologies, architectures, protocols, and clouds. 

To let you set up and leverage advanced routing, traffic management, security, resiliency, integration, and AI capabilities, you can choose to run a Gloo Gateway proxy with one of the following APIs: the **Kubernetes Gateway API with custom Gloo Gateway extensions** or the **Gloo Edge API**. The Gloo Gateway control plane can process custom resource requests from both APIs and translate these resources into valid Envoy configuration that the Envoy proxies can pick up and apply.

Because of that, Gloo Gateway gives you the flexibility to run both Kubernetes Gateway API and Gloo Edge API Envoy proxies in your environment at the same time so that you can customize your API gateway to the needs of your app and organization

[**Installation for K8s Gateway API**](https://docs.solo.io/gateway/latest/quickstart/) &nbsp; |
&nbsp; [**Installation for Gloo Edge API**](https://docs.solo.io/gloo-edge/latest/getting_started/) &nbsp; |
&nbsp; [**K8s Gateway API docs**](https://docs.solo.io/gateway/latest/) &nbsp; |
&nbsp; [**Gloo Edge API docs**](https://docs.solo.io/gateway/latest/) &nbsp; |
&nbsp; [**Blog**](https://www.solo.io/blog/?category=gloo) &nbsp; |
&nbsp; [**Slack**](https://slack.solo.io) &nbsp; |
&nbsp; [**Twitter**](https://twitter.com/soloio_inc) |
&nbsp; [**Enterprise Trial**](https://www.solo.io/free-trial/)

<BR><center><img src="https://docs.solo.io/gateway/img/gateway/gloo-gateway-ov.svg" alt="Gloo Gateway Architecture" width="700"></center>

### Using Gloo Gateway
- **Kubernetes Gateway API**: Gloo Gateway is a feature-rich ingress controller, built on top of the Envoy Proxy and fully conformant with the Kubernetes Gateway API.
- **Next-generation API gateway**: Gloo Gateway provides a long list of API gateway features including rate limiting, circuit breaking, retries, caching, transformation, service-mesh integration, security, external authentication and authorization.
- **Hybrid apps**: Gloo Gateway creates applications that route to backends implemented as microservices, serverless functions and legacy apps. This feature can help users to
  * Gradually migrate from their legacy code to microservices and serverless.
  * Add new functionalities using cloud-native technologies while maintaining their legacy codebase.
  * Allow different teams in an organization choose different architectures. 


### What makes Gloo Gateway unique
- **Function-level routing allows integration of legacy applications, microservices and serverless**: Gloo Gateway can route requests directly to functions. Request to Function can be a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS Function, etc.), an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.), or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes Gloo Gateway the only API gateway that supports hybrid apps as well as the only one that does not tie the user to a specific paradigm.
- **Gloo Gateway incorporates vetted open-source projects to provide broad functionality**: Gloo Gateway supports high-quality features by integrating with top open-source projects, including gRPC, GraphQL, OpenTracing, NATS and more. Gloo Gateway's architecture allows rapid integration of future popular open-source projects as they emerge.
- **Full automated discovery lets users move fast**: Upon launch, Gloo Gateway creates a catalog of all available destinations and continuously keeps them up to date. This takes the responsibility for 'bookkeeping' away from the developers and guarantees that new features become available as soon as they are ready. Gloo Gateway discovers across IaaS, PaaS and FaaS providers as well as Swagger, gRPC, and GraphQL.


## Next Steps
- Join us on our Slack channel: [https://slack.solo.io/](https://slack.solo.io/)
- Follow us on Twitter: [https://twitter.com/soloio_inc](https://twitter.com/soloio_inc)
- Check out the docs: [https://docs.solo.io/gateway/latest/](https://docs.solo.io/gateway/latest/)

## Contributing to Gloo Gateway
The [devel](devel) folder should be the starting point for understanding the code, and contributing to the product.

## Thanks
**Gloo Gateway** would not be possible without the valuable open-source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io).


## Security
*Reporting security issues* : We take Gloo Gateway's security very seriously. If you've found a security issue or a potential security issue in Gloo Gateway, please DO NOT file a public Github issue, instead send your report privately to [security@solo.io](mailto:security@solo.io).
