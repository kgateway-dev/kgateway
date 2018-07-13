<a name="top"></a>

## Contents
  - [Attribute](#gloo.api.v1.Attribute)
  - [ListenerAttribute](#gloo.api.v1.ListenerAttribute)



<a name="attribute"></a>
<p align="right"><a href="#top">Top</a></p>




<a name="gloo.api.v1.Attribute"></a>

### Attribute
An attribute is a container for configuration that is intended to be applied across a set of labeled resources inside of Gloo.
Attributes specify a set of selectors which are compared with labels by Gloo at runtime
In the current implementation, only Listeners have be selected, and therefore configured by Attributes.
Labels and Selectors follow the same logical patterns implemented by Kubernetes.
Read about the Kubernetes concepts here: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
Attributes are useful when applying shared configuration to a large number of objects, such as the sharing of route
configuration between roles.


```yaml
name: string
listener_attribute: {ListenerAttribute}
status: (read only)
metadata: {Metadata}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | string |  | Name of the attribute. Attribute Names must be unique and follow the following syntax rules: One or more lowercase rfc1035/rfc1123 labels separated by &#39;.&#39; with a maximum length of 253 characters. |
| listener_attribute | [ListenerAttribute](attribute.md#gloo.api.v1.ListenerAttribute) |  |  |
| status | [Status](status.md#gloo.api.v1.Status) |  | Status indicates the validation status of the attribute resource. Status is read-only by clients, and set by gloo during validation |
| metadata | [Metadata](metadata.md#gloo.api.v1.Metadata) |  | Metadata contains the resource metadata for the attribute |






<a name="gloo.api.v1.ListenerAttribute"></a>

### ListenerAttribute
Listeners define the address:port where the proxy will listen for incoming connections
Each listener defines a unique set of TCP and HTTP behaviors


```yaml
selector: map<string,string>
virtual_services: [string]
config: {google.protobuf.Struct}

```
| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| selector | map&lt;string,string&gt; |  | Apply this attribute to listeners with label keys and values matching this selector. If empty or not present, the attribute will not be applied to any listeners. |
| virtual_services | string | repeated | Listeners can serve HTTP or raw TCP, but not both. If at least one Virtual Service is specified here, the listener will become an HTTP listener serving routes defined in these virtual services. Some Listener plugins may impose restrictions on the Virtual Services that can be applied to a listener. For example, some plugins may require all applied virtual services only route to a specific upstream, a common requirement for Service Meshes |
| config | [google.protobuf.Struct](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/struct) |  | Config contains top-level config to be applied to a listener Listener config is applied to all TCP/HTTP traffic that initiates via this listener. Configuration such as gzip compression and TLS authentication is specified here This config struct will be merged with Role-specific Listener Conig. If two fields overlap between the Listener config on the role and the config on the attribute, the config on the Role will supersede this one |





 

 

 

