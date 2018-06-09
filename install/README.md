
<h1 align="center">
    <img src="Gloo-01.png" alt="Gloo" width="200" height="242">
  <br>
</h1>


<h3 align="center">Gloo Installation</h3>
<BR>

Gloo is a function gateway built on top of the [Envoy Proxy](https://www.Envoyproxy.io). Gloo provides a unified entry point
for access to all services and serverless functions, translating from any interface spoken by a client to any interface
spoken by a backend. Gloo aggregates REST APIs and events calls from clients, "glueing" together services in-cluster, 
out of cluster, across clusters, along with any provider of serverless functions.

This Repo 
----
Here you will find installation scripts, files, and charts for various deployment strategies for Gloo.

The simplest and most well-tested of these lives at `kube/install.yaml`. Try deploying this to a kubernetes cluster with
```bash
kubectl apply \
  --filename https://raw.githubusercontent.com/solo-io/gloo/master/install/kube/install.yaml
```
to get started quickly.

See our [documentation](https://gloo.solo.io) for instructions on installation and getting started with Gloo. 

Documentation
-----

Get started by reading our docs here: [https://gloo.solo.io/](https://gloo.solo.io/)

Community
-----
Join us on our slack channel: [https://slack.solo.io/](https://slack.solo.io/)
