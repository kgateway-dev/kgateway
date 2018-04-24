<a name="top"></a>

## Contents
  - [Config](#v1.Config)



<a name="config"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="v1.Config"></a>

### Config
Config is a top-level config object. It is used internally by gloo as a container for the entire user config.


```yaml
upstreams: [{Upstream}]
virtual_services: [{VirtualService}]

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| upstreams | [Upstream](upstream.md#v1.Upstream) | repeated | The list of all upstreams defined by the user. |
| virtual_services | [VirtualService](virtualservice.md#v1.VirtualService) | repeated | the list of all virtual services defined by the user. |





 

 

 

