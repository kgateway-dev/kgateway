# Service Plugin for Gloo


#### Description

The [Static Plugin for Gloo](https://github.com/solo-io/gloo/tree/master/pkg/coreplugins/static) is a basic plugin enabling
routing to an upstream which is simply a list of host/port combinations for a single service. 
A typical use case for defining `static` upstreams is to route to external services, or route to a service whose upstream
type is not yet supported by an existing Gloo plugin.


#### Upstream Spec Configuration

The **Upstream Type** for service upstreams is `static`. 

The [upstream spec](../v1/upstream.md#v1.Upstream) for Service Upstreams has the following structure:

```yaml
hosts:
- addr: 10.137.22.200
  port: 8080
- addr: some-host.example.com
  port: 1234
```

| Field | Type |  Description |
| ----- | ---- |  ----------- |
| hosts | []Host |  a list of Hosts to which routes for this upstream should connect. **at least one required** |

A Host has the following structure:

| addr | string |  an IP or Hostname for the upstream. **required** |
| port | int | the port on which to reach the upstream

#### Example Service Upstream

The following is an example of a valid Service [Upstream](../introduction/concepts.md#Upstreams):

```yaml
name: my-external-service
spec:
  hosts:
  - addr: 10.137.22.200
    port: 8080
  - addr: some-host.example.com
    port: 1234
type: static
```
