<a name="top"></a>

## Contents
  - [Status](#v1.Status)

  - [Status.State](#v1.Status.State)


<a name="status"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="v1.Status"></a>

### Status
Status indicates whether a config resource (currently only [virtualservices](../introduction/concepts.md) and [upstreams](../introduction/concepts.md)) has been (in)validated by gloo


```yaml
state: {Status.State}
reason: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| state | [Status.State](status.md#v1.Status.State) |  | State is the enum indicating the state of the resource |
| reason | string |  | Reason is a description of the error for Rejected resources. If the resource is pending or accepted, this field will be empty |





 


<a name="v1.Status.State"></a>

### Status.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| Pending | 0 | Pending status indicates the resource has not yet been validated |
| Accepted | 1 | Accepted indicates the resource has been validated |
| Rejected | 2 | Rejected indicates an invalid configuration by the user Rejected resources may be propagated to the xDS server depending on their severity |


 

 

