<a name="top"></a>

## Contents
  - [Role](#v1.Role)



<a name="role"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="v1.Role"></a>

### Role
A Role is a container for a set of Virtual Services that will be used to generate a single proxy config
to be applied to one or more Envoy nodes. The Role is best understood as an in-mesh application&#39;s localized view
of the rest of the mesh.
Each domain for each Virtual Service contained in a Role cannot appear more than once, or the Role
will be invalid.
In the current implementation, Roles are read-only objects created by Gloo for the puprose of reporting.
In the future, Gloo will support fields in Roles that can be written to for the purpose of applying policy
to groups of Virtual Services.


```yaml
name: string
virtual_services: [string]
status: (read only)
metadata: {Metadata}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | string |  | Name of the role. Envoy nodes will be assigned a config matching the role they report to Gloo when registering Envoy instances must specify their role in the prefix for their Node ID when they register to Gloo. Currently this is done in the format &lt;Role&gt;~&lt;this portion is ignored&gt; which can be specified with the `--service-node` flag, or in the Envoy instance&#39;s bootstrap config. Role Names must be unique and follow the following syntax rules: One or more lowercase rfc1035/rfc1123 labels separated by &#39;.&#39; with a maximum length of 253 characters. |
| virtual_services | string | repeated | a list of virtual services that reference this role |
| status | [Status](status.md#v1.Status) |  | Status indicates the validation status of the role resource. Status is read-only by clients, and set by gloo during validation |
| metadata | [Metadata](metadata.md#v1.Metadata) |  | Metadata contains the resource metadata for the role |





 

 

 

