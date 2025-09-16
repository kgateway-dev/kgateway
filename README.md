<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo-dark.svg" alt="kgateway" width="400">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo.svg" alt="kgateway" width="400">
    <img alt="kgateway" src="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo.svg">
  </picture>
  <br/>
  An Envoy-Powered, Kubernetes-Native API Gateway
</h1>

[![OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/projects/10534/badge)](https://bestpractices.coreinfrastructure.org/projects/10534)

## About kgateway

Kgateway is:

* **An ingress/edge router for Kubernetes**: Powered by [Envoy](https://www.envoyproxy.io) and programmed with the [Gateway API](https://gateway-api.sigs.k8s.io/), kgateway is a world-leading Cloud Native ingress.
* **An advanced API gateway**: Aggregate web APIs and apply key functions like authentication, authorization and rate limiting in one place
* **A better waypoint proxy for [ambient mesh](https://ambientmesh.io/)**: Use the same stack for east-west management as you do for north-south.
* **An AI gateway for securing LLM usage**: Protect applications, models, and data from inappropriate access or use, whether you're producing or consuming. Manage traffic to LLM providers, and enrich prompts at a system level.
* **An LLM Gateway utilizing the [Inference Extension](https://gateway-api-inference-extension.sigs.k8s.io/) project**: Intelligently route to AI inference workloads and LLMs in your Kubernetes environment.
* **A model context protocol (MCP) gateway**: Federate MCP tool servers into a single, scalable and secure endpoint.
* **A migration engine for hybrid apps**: Route to backends implemented as microservices, serverless functions or legacy apps. This can help you gradually migrate from legacy code to microservices and serverless, add new functionalities using cloud-native technologies while maintaining a legacy codebase or allow different teams in an organization to choose different architectures.

Kgateway is feature-rich, fast, and flexible. It excels in function-level routing, supports legacy apps, microservices and serverless, offers robust discovery capabilities, integrates seamlessly with open-source projects, and is designed to support hybrid applications with various technologies, architectures, protocols, and clouds.

The project was previously known as Gloo, and has been [production-ready since 2019](https://www.solo.io/blog/announcing-gloo-1-0-a-production-ready-envoy-based-api-gateway). Please see [the migration plan](https://github.com/kgateway-dev/kgateway/issues/10363) for more information and the current status of the change from Gloo to kgateway.

## Get involved

- [Join us on our Slack channel](https://kgateway.dev/slack/)
- [Check out the docs](https://kgateway.dev/docs)
- [Read the kgateway blog](https://kgateway.dev/blog/)
- [Learn more about the community](https://github.com/kgateway-dev/community)
- [Watch a video on our YouTube channel](https://www.youtube.com/@kgateway-dev)
- Follow us on [X](https://x.com/kgatewaydev), [Bluesky](https://bsky.app/profile/kgateway.dev), [Mastodon](https://mastodon.social/@kgateway) or [LinkedIn](https://www.linkedin.com/company/kgateway/)

## Contributing to kgateway

Please refer to [devel/contributing/README.md](/devel/contributing/README.md) as a starting point for contributing to the project.

## Releasing kgateway

Please refer to [devel/contributing/releasing.md](devel/contributing/releasing.md) as a starting point for understanding releases of the project.

## Thanks

Kgateway would not be possible without the valuable open source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io), upon whose shoulders we stand.

## Security

*Reporting security issues* : We take kgateway's security very seriously. If you've found a security issue or a potential security issue in kgateway, please DO NOT file a public GitHub issue. Instead follow [the directions laid out in the kgateway/community repository](https://github.com/kgateway-dev/community/blob/main/CVE.md).

---

<div align="center">
    <img src="https://raw.githubusercontent.com/cncf/artwork/main/other/cncf-sandbox/horizontal/color/cncf-sandbox-horizontal-color.svg" width="300" alt="Cloud Native Computing Foundation logo"/>
    <p>kgateway is a <a href="https://cncf.io">Cloud Native Computing Foundation</a> sandbox project.</p>
</div>
