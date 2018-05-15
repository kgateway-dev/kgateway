<a name="top"></a>

## Contents
  - [Report](#v1.Report)
  - [ObjectReference](#v1.ObjectReference)
  - [Status](#v1.Status)

  - [ObjectReference.ObjectType](#v1.ObjectReference.ObjectType)
  - [Status.State](#v1.Status.State)


<a name="report"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="v1.Report"></a>

### Report
A Report contains config validation information for users.
Gloo generates reports for every top-level config object (VirtualServices and Upstreams)
indicating whether the resource was accepted by Gloo or rejected (due to an invalid configuration).


```yaml
id: string
object_reference: {ObjectReference}
status: (read only)

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | string |  | a unique identifier for the report |
| object_reference | [ObjectReference](report.md#v1.ObjectReference) |  | reference to the object this report pertains to |
| status | [Status](report.md#v1.Status) |  | Status describes the status of the object |






<a name="v1.ObjectReference"></a>

### ObjectReference



```yaml
object_type: {ObjectReference.ObjectType}
name: string
namespace: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| object_type | [ObjectReference.ObjectType](report.md#v1.ObjectReference.ObjectType) |  | type of the object can be Upstream or VirtualService |
| name | string |  | name of the object |
| namespace | string |  | optional namespace of the object |






<a name="v1.Status"></a>

### Status
Status indicates whether a config resource has been (in)validated by gloo


```yaml
state: {Status.State}
reason: string

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| state | [Status.State](report.md#v1.Status.State) |  | State is the enum indicating the state of the resource |
| reason | string |  | Reason is a description of the error for Rejected resources. If the resource is pending or accepted, this field will be empty |





 


<a name="v1.ObjectReference.ObjectType"></a>

### ObjectReference.ObjectType


| Name | Number | Description |
| ---- | ------ | ----------- |
| Upstream | 0 |  |
| VirtualService | 1 |  |



<a name="v1.Status.State"></a>

### Status.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| Pending | 0 | Pending status indicates the resource has not yet been validated |
| Accepted | 1 | Accepted indicates the resource has been validated |
| Rejected | 2 | Rejected indicates an invalid configuration by the user Rejected resources may be propagated to the xDS server depending on their severity |


 

 

