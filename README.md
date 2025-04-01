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

Kgateway is:

* **An ingress/edge router for Kubernetes**: Powered by [Envoy](https://www.envoyproxy.io) and programmed with the [Gateway API](https://gateway-api.sigs.k8s.io/), kgateway is a world-leading Cloud Native ingress.
* **An advanced API gateway**: Aggregate web APIs and apply key functions like authentication, authorization and rate limiting in one place
* **A better waypoint proxy for [ambient mesh](https://ambientmesh.io/)**: Use the same stack for east-west management as you do for north-south.
* **An AI gateway for securing LLMs**: Protect applications, models, and data from inappropriate access, whether you're producing or consuming.
* **A migration engine for hybrid apps**: Toute to backends implemented as microservices, serverless functions or legacy apps. This can help you gradually migrate from legacy code to microservices and serverless, add new functionalities using cloud-native technologies while maintaining a legacy codebase or allow different teams in an organization to choose different architectures.

Kgateway is feature-rich, fast, and flexible. It excels in function-level routing, supports legacy apps, microservices and serverless, offers robust discovery capabilities, integrates seamlessly with open-source projects, and is designed to support hybrid applications with various technologies, architectures, protocols, and clouds.

The project was previously known as Gloo, and has been [production-ready since 2019](https://www.solo.io/blog/announcing-gloo-1-0-a-production-ready-envoy-based-api-gateway).

Please see [the migration plan](https://github.com/kgateway-dev/kgateway/issues/10363) for more information and the current status of the change from Gloo to kgateway.

[**Installation**](https://kgateway.dev/docs/quickstart/) &nbsp; |
&nbsp; [**Documentation**](https://kgateway.dev/docs) &nbsp; |
&nbsp; [**Blog**](https://kgateway.dev/blog/) &nbsp; |
&nbsp; [**Slack**](https://kgateway.dev/slack/)

<!---
PLEASE DO NOT RENAME THIS SECTION
This header is used as an anchor in our CNCF Donation Issue
-->
### What makes kgateway unique
- **Function-level routing allows integration of legacy applications, microservices and serverless**: Kgateway can route requests directly to functions. Request to Function can be a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS Function, etc.), an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.), or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes kgateway the only API gateway that supports hybrid apps as well as the only one that does not tie the user to a specific paradigm.
- **Kgateway incorporates vetted open-source projects to provide broad functionality**: Kgateway supports high-quality features by integrating with top open-source projects, including gRPC, OpenTracing, NATS and more. Kgateway's architecture allows rapid integration of future popular open-source projects as they emerge.
- **Full automated discovery lets users move fast**: Upon launch, kgateway creates a catalog of all available destinations and continuously keeps them up to date. This takes the responsibility for 'bookkeeping' away from the developers and guarantees that new features become available as soon as they are ready. Kgateway discovers across IaaS, PaaS and FaaS providers as well as Swagger, and gRPC.

## Next steps
- [Join us on our Slack channel](https://kgateway.dev/slack/)
- [Check out the docs](https://kgateway.dev/docs)
- [Learn more about the community](https://github.com/kgateway-dev/community)

## Contributing to kgateway
The [devel](devel) folder should be the starting point for understanding the code, and contributing to the product.

## Thanks
Kgateway would not be possible without the valuable open source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io), upon whose shoulders we stand.

## Security
*Reporting security issues* : We take kgateway's security very seriously. If you've found a security issue or a potential security issue in kgateway, please DO NOT file a public GitHub issue. Instead follow [the directions laid out in the kgateway/community repository](https://github.com/kgateway-dev/community/blob/main/CVE.md).

---

<div align="center">
    <img src="https://raw.githubusercontent.com/cncf/artwork/main/other/cncf-sandbox/horizontal/color/cncf-sandbox-horizontal-color.svg" width="300" alt="Cloud Native Computing Foundation logo"/>
    <p>kgateway is a <a href="https://cncf.io">Cloud Native Computing Foundation</a> sandbox project.</p>
</div>