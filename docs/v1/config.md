<a name="top"></a>

## Contents
  - [Config](#gloo.api.v1.Config)



<a name="config"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.v1.Config"></a>

### Config
Config is a top-level config object. It is used internally by gloo as a container for the entire set of config objects.


```yaml
upstreams: [{Upstream}]
virtual_services: [{VirtualService}]
roles: [{Role}]
attributes: [{Attribute}]

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| upstreams | [Upstream](upstream.md#gloo.api.v1.Upstream) | repeated | The list of all upstreams defined by the user. |
| virtual_services | [VirtualService](virtualservice.md#gloo.api.v1.VirtualService) | repeated | the list of all virtual services defined by the user. |
| roles | [Role](role.md#gloo.api.v1.Role) | repeated | the list roles defined by the user |
| attributes | [Attribute](attribute.md#gloo.api.v1.Attribute) | repeated | the list attributes defined by the user |





 

 

 

