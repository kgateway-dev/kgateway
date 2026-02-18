<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo-dark.svg" alt="kgateway" width="400">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo.svg" alt="kgateway" width="400">
    <img alt="kgateway" src="https://raw.githubusercontent.com/kgateway-dev/kgateway.dev/main/static/logo.svg">
  </picture>
  <br/>
  The most widely deployed gateway in Kubernetes for microservices
</h1>
<div align="center">
  <a href="https://github.com/kgateway-dev/kgateway/releases">
    <img src="https://img.shields.io/github/v/release/kgateway-dev/kgateway?style=flat&label=Latest%20version" alt="Release">
  </a>
  <a href="https://opensource.org/licenses/Apache-2.0">
    <img src="https://img.shields.io/badge/License-Apache2.0-brightgreen.svg?style=flat" alt="License: Apache 2.0">
  </a>
  <a href="https://github.com/kgateway-dev/kgateway">
    <img src="https://img.shields.io/github/stars/kgateway-dev/kgateway.svg?style=flat&logo=github&label=Stars" alt="Stars">
  </a>
  <a href="https://www.bestpractices.dev/projects/10534"><img src="https://www.bestpractices.dev/projects/10534/badge" alt="OpenSSF Best Practices"></a>
</div>

## About kgateway

Kgateway is the most mature and widely deployed gateway in the market today. Built on open source and open standards, **kgateway is a resilient and performance-oriented control plane that implements the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/) for [Envoy](https://github.com/envoyproxy/envoy)**.

With a control plane that scales from lightweight microgateway deployments between services to massively parallel centralized gateways handling billions of API calls, kgateway brings omni-directional API connectivity to any cloud and any environment.

### Use Cases

Kgateway is designed for:

* **Next-Gen API Gateway**: Expose APIs and apply policies like authentication, authorization and rate limiting in one place. Powered by [Envoy](https://www.envoyproxy.io) and programmed with the [Gateway API](https://gateway-api.sigs.k8s.io/), kgateway is a world-leading Cloud Native API Gateway.

* **Hybrid Application Migration**: Route to backends implemented as microservices, serverless functions or legacy apps. Gradually migrate from legacy code while maintaining existing systems.

* **Complex Tenancy Environments**: Multiple personas and teams can work with kgateway to express their APIs, with concepts like route delegation and composable policies.

Kgateway is feature-rich, fast, and flexible. It excels in function-level routing, supports legacy apps, microservices and serverless, offers robust discovery capabilities, integrates seamlessly with open-source projects, and is designed to support hybrid applications with various technologies, architectures, protocols, and clouds.

> [!IMPORTANT]
> Kgateway previously acted as a control plane for the agentgateway dataplane to enable several AI and agentic features.
> Starting with version 2.3.0, the control plane for agentgateway has been migrated to the [agentgateway](https://github.com/agentgateway/agentgateway) repo, enabling a singular focus for kgateway to be a stable, robust, and battle-tested API Gateway powered by Envoy.

### History

The project was launched in 2018 as **Gloo** by Solo.io and has been [production-ready since 2019](https://www.solo.io/blog/announcing-gloo-1-0-a-production-ready-envoy-based-api-gateway). Since then, it has steadily evolved to become the most trusted and feature-rich API gateway for Kubernetes, processing billions of API requests for many of the world's biggest companies. Please see [the migration plan](https://github.com/kgateway-dev/kgateway/issues/10363) for more information about the transition from Gloo to kgateway.

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

## Security

See our [SECURITY.md](SECURITY.md) file for details.

## Thanks

Kgateway would not be possible without the valuable open source work of projects in the community. We would like to extend a special thank-you to [Envoy](https://www.envoyproxy.io) and [agentgateway](https://github.com/agentgateway/agentgateway), the two data planes upon which we build our dual control plane architecture.

## Contributors

Thanks to all contributors who are helping to make kgateway better!

<a href="https://github.com/kgateway-dev/kgateway/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=kgateway-dev/kgateway" />
</a>

## Star History

<a href="https://www.star-history.com/#kgateway-dev/kgateway&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=kgateway-dev/kgateway&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=kgateway-dev/kgateway&type=Date" />
   <img alt="Star history of kgateway-dev/kgateway over time" src="https://api.star-history.com/svg?repos=kgateway-dev/kgateway&type=Date" />
 </picture>
</a>

---

<div align="center">
    <img src="https://raw.githubusercontent.com/cncf/artwork/main/other/cncf-sandbox/horizontal/color/cncf-sandbox-horizontal-color.svg" width="300" alt="Cloud Native Computing Foundation logo"/>
    <p>kgateway is a <a href="https://cncf.io">Cloud Native Computing Foundation</a> sandbox project.</p>
</div>
