<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo-dark.svg" alt="kgateway" width="400">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo.svg" alt="kgateway" width="400">
    <img alt="kgateway" src="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo.svg">
  </picture>
  <br/>
  An Envoy-Powered, Kubernetes-Native API Gateway
</h1>

## About kgateway
Kgateway is a feature-rich, fast, and flexible Kubernetes-native ingress controller and next-generation API gateway that is built on top of [Envoy proxy](https://www.envoyproxy.io) and the Kubernetes Gateway API. It excels in function-level routing, supports legacy apps, microservices and serverless, offers robust discovery capabilities, integrates seamlessly with open-source projects, and is designed to support hybrid applications with various technologies, architectures, protocols, and clouds.

Please see [the plan](https://github.com/kgateway-dev/kgateway/issues/10363) for more information and current status.

[**Installation**](https://kgateway.dev/docs/quickstart/) &nbsp; |
&nbsp; [**Documentation**](https://kgateway.dev/docs) &nbsp; |
&nbsp; [**Blog**](https://kgateway.dev/blog/) &nbsp; |
&nbsp; [**Slack invite**](https://slack.cncf.io/) &nbsp; |
&nbsp; [**Slack channel**](https://cloud-native.slack.com/archives/C080D3PJMS4)

<BR><center><img align="center" src="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/assets/img/component-architecture.svg" alt="kgateway Architecture" width="700"></center>

### Using kgateway
- **Kubernetes Gateway API**: Kgateway is a feature-rich ingress controller, built on top of the Envoy Proxy and fully conformant with the Kubernetes Gateway API.
- **Next-generation API gateway**: Kgateway provides a long list of API gateway features including rate limiting, circuit breaking, retries, caching, transformation, service-mesh integration, security, external authentication and authorization.
- **Hybrid apps**: Kgateway creates applications that route to backends implemented as microservices, serverless functions and legacy apps. This feature can help users to
  * Gradually migrate from their legacy code to microservices and serverless.
  * Add new functionalities using cloud-native technologies while maintaining their legacy codebase.
  * Allow different teams in an organization choose different architectures.

<!---
PLEASE DO NOT RENAME THIS SECTION
This header is used as an anchor in our CNCF Donation Issue
-->
### What makes kgateway unique
- **Function-level routing allows integration of legacy applications, microservices and serverless**: Kgateway can route requests directly to functions. Request to Function can be a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS Function, etc.), an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.), or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes kgateway the only API gateway that supports hybrid apps as well as the only one that does not tie the user to a specific paradigm.
- **Kgateway incorporates vetted open-source projects to provide broad functionality**: Kgateway supports high-quality features by integrating with top open-source projects, including gRPC, OpenTracing, NATS and more. Kgateway's architecture allows rapid integration of future popular open-source projects as they emerge.
- **Full automated discovery lets users move fast**: Upon launch, kgateway creates a catalog of all available destinations and continuously keeps them up to date. This takes the responsibility for 'bookkeeping' away from the developers and guarantees that new features become available as soon as they are ready. Kgateway discovers across IaaS, PaaS and FaaS providers as well as Swagger, and gRPC.


## Next steps
- Join us on our Slack channel: [#kgateway](https://cloud-native.slack.com/archives/C080D3PJMS4) ([get an invite here]((https://slack.cncf.io/)))
- Check out the docs: [https://kgateway.dev/docs](https://kgateway.dev/docs)
- Learn more about the [community](https://github.com/kgateway-dev/community)

## Contributing to kgateway
The [devel](devel) folder should be the starting point for understanding the code, and contributing to the product.

## Thanks
Kgateway would not be possible without the valuable open-source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io).

## Security
*Reporting security issues* : We take kgateway's security very seriously. If you've found a security issue or a potential security issue in kgateway, please DO NOT file a public GitHub issue. Instead follow [the directions laid out in the kgateway/community repository](https://github.com/kgateway-dev/community/blob/main/CVE.md).
