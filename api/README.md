
<h1 align="center">
    <img src="/docs/Gloo-01.png" alt="Gloo" width="200" height="242">
  <br>
</h1>


<h3 align="center">API Definitions for Gloo</h3>
<BR>

Gloo is a function gateway built on top of the [Envoy Proxy](https://www.Envoyproxy.io). Gloo provides a unified entry point
for access to all services and serverless functions, translating from any interface spoken by a client to any interface
spoken by a backend. Gloo aggregates REST APIs and events calls from clients, "glueing" together services in-cluster, 
out of cluster, across clusters, along with any provider of serverless functions.

This Repo 
----
Here you will find the `.proto` files that define the base for Gloo's configuration language. Extensions to the language
are defined in Gloo's extensive plugin ecosystem. See the [plugins](/pkg/plugins)
and our [documentation](https://gloo.solo.io) for more information about plugin-specific API, as well as extending the
Gloo language.

Documentation
-----

Get started by reading our docs here: [https://gloo.solo.io/](https://gloo.solo.io/)

Community
-----
Join us on our slack channel: [https://slack.solo.io/](https://slack.solo.io/)
