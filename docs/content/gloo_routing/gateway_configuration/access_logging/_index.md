---
title: Access Logging
weight: 1
---

Gloo can be configured to provide extensive Access Logging from envoy. These logs can be configured for 
HTTP (L7) connections as well at TCP (L4).


# Access Logging

The envoy documentation on Access Logging can be found 
[here](https://www.envoyproxy.io/docs/envoy/v1.10.0/configuration/access_log#config-access-log-default-format).

#### Usage

Access Logging allows for more verbose, customizable usage logs from envoy. These logs will not replace the normal logs outputted by envoy, but can be used instead to supplement them. 
Possible use cases include:

*  specially formatted string logs
*  formatted JSON logging to be ingested by log aggregators
*  GRPC streaming of logs to external services

#### Configuration

The following explanation assumes that the user has gloo `v0.18.1+` running, as well as some previous knowledge of Gloo resources, and how to use them. In order to install Gloo if it is not already please refer to the following [tutorial](../../../installation/gateway/kubernetes). The only Gloo resource involved in enabling Access Loggins is the `Gateway`. Further Documentation can be found {{< protobuf name="gateway.solo.io.Gateway" display="here">}}.

Enabling access logs in Gloo is as simple as adding a [listener plugin](../../gateway_configuration/) to any one of the gateway resources. 
The documentation for the `Access Logging Service` plugin API can be found {{< protobuf display="here" name="als.options.gloo.solo.io.AccessLog">}}.

Gloo supports two types of Access Logging. `File Sink` and `GRPC`.

### File Sink

Within the `File Sink` category of Access Logs there are 2 options for output, those being:

* [String formatted](#string-formatted)
* [JSON formatted](#json-formatted)

These are mutually exclusive for a given Access Logging configuration, but any number of access logging configurations can be applied to any place in the API which supports Access Logging. All `File Sink` configurations also accept a file path which envoy logs to. If the desired behavior is for these logs to output to `stdout` along with the other envoy logs then use the value `/dev/stdout` as the path.

The documentation on envoy formatting directives can be found [here](https://www.envoyproxy.io/docs/envoy/v1.10.0/configuration/access_log#format-dictionaries)

{{% notice note %}}
See [**this guide**]({{< ref "gloo_routing/virtual_services/routes/routing_features/transformations/enrich_access_logs" >}}) 
to see how to include custom attributes in your access logs by leveraging Gloo's 
[**transformation API**]({{< ref "gloo_routing/virtual_services/routes/routing_features/transformations" >}}).
{{% /notice %}}

##### String formatted

An example config for string formatted logs is as follows:
{{< highlight yaml "hl_lines=15-20" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  annotations:
    origin: default
  name: gateway
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames: 
    - gateway-proxy
  httpGateway: {}
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          stringFormat: ""
{{< / highlight >}}


The above yaml also includes the Gateway object it is contained in. Notice that the `stringFormat` field above is set to `""`. This is intentional. If the string is set to `""` envoy will use a standard formatting string. More information on this as well as how to create a customized string see [here](https://www.envoyproxy.io/docs/envoy/v1.10.0/configuration/access_log#default-format-string).

##### JSON formatted

An example config for JSON formatted logs is as follows:

{{< highlight yaml "hl_lines=15-22" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  annotations:
    origin: default
  name: gateway
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  proxyNames: 
    - gateway-proxy
  httpGateway: {}
  useProxyProto: false
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          jsonFormat:
            protocol: "%PROTOCOL%"
            duration: "%DURATION%"
{{< / highlight >}}

The majority is the same as the above, as the gateway has the same config, the differece exists in the formatting of the file sink. Instead of a simple string formatting directive, this config accepts an object value which is transformed by envoy into JSON formatted logs. The object inside of the `jsonFormat` field is interpreted as a JSON object. This object consists of nested json objects as well as keys which point to individual formatting directives. More documentation on JSON formatting can be found [here](https://www.envoyproxy.io/docs/envoy/v1.10.0/configuration/access_log#format-dictionaries).


## GRPC Access Logging

To access the GRPC Access logging feature Gloo `v0.18.38+` is required.

Gloo now supports Envoy GRPC access logging. Logging access data directly to a file can be very useful, but sometimes collecting the data via a GRPC service can be a better fit. 
The best example of such a situation is when the access logging data is useful or required by other microservices. File Sink has filtering options, but programmatic filtering, as well as 
eventing with the GRPC data can be very powerful, and allow for unique features to be built using the data being streamed.

GRPC Access logging functions very similarly to the file sink, however there are no formatting directives, as all of the data is sent via GRPC requests, and management of said data falls to the user.
There are only two configuration steps needed to get started:

 * Add a cluster to the envoy bootstrap config which points to the access-logging service.
 * Pass a reference to the cluster into the access logging API

These instructions assume that a service already exists at the predefined location which is listening for access logging grpc connections. In order to make is easier to get up and running, we provide
a simnple implementation which simply receives the messages and then logs them. The code for this implementation can be found [here](https://github.com/solo-io/gloo/tree/master/projects/accesslogger/pkg/loggingservice).
The server itself is extendable, and can be ran with callbacks if different behavior is needed. The implementation referenced above is included in our helm chart, which is included in the manifest when
the access logger is enabled. In order to use a different service, simply swap the image name in the helm chart.

In order to demonstrate the GRPC access logging in action, we are going to run the latest gloo as well as the petstore demo.
To install Gloo we will be using `glooctl`

Firstly, save the following values file locally:
```yaml
accessLogger:
  enabled: true
```
There are more options available, but this is the simplest to get up and running. Once this is saved locally we can install gloo.
Save the location of this file to the env variable `LOCAL_VALUES_FILE`

Using glooctl:
```bash
glooctl install gateway -n gloo-system --values $LOCAL_VALUES_FILE
```

Now run
```bash
kubectl get pods -n gloo-system

NAME                                              READY   STATUS    RESTARTS   AGE
api-server-5c46c77c9-9724f                        3/3     Running   0          2m49s
discovery-56dcb649c8-4m9d4                        1/1     Running   0          2m49s
gateway-proxy-59c46d569-lwhbh                  1/1     Running   0          2m49s
gateway-proxy-access-logger-6bb9f97fb8-8v8h8   1/1     Running   0          2m49s
gateway-58c58fcd46-vccmt                       1/1     Running   0          2m49s
gloo-7975c97546-ssh26                             1/1     Running   0          2m49s
```
The output should be similar to the above, minus the generated section of the names.

Once all of the gloo pods are up and running let's go ahead and install the petstore. The tutorial on how to do this as well as basic gloo routing is located [here](../../hello_world/).
Once the petstore pod is up and running we will route some traffic to it, and test that the traffic is being recorded in the access logs.

Run the following curl from the routing tutorial doc.
```bash
curl $(glooctl proxy url)/sample-route-1
```

returns

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Once this command has been run successfully let's go check the access logs to see that it has been delivered and reported.

```bash
kubectl get logs -n gloo-system deployments/gateway-proxy-access-logger | grep /api/pets
{"level":"info","ts":"2019-09-09T17:56:52.669Z","logger":"access_log","caller":"runner/run.go:50","msg":"received http request","logger_name":"test","node_id":"gateway-proxy-59c46d569-kmjhb.gloo-system","node_cluster":"gateway","node_locality":"<nil>","node_metadata":"&Struct{Fields:map[string]*Value{role: &Value{Kind:&Value_StringValue{StringValue:gloo-system~gateway-proxy,},XXX_unrecognized:[],},},XXX_unrecognized:[],}","protocol_version":"HTTP11","request_path":"/api/pets","request_method":"GET","response_status":"&UInt32Value{Value:200,XXX_unrecognized:[],}"}
```

If all went well this command should yield all of the requests whose request path includes `/api/pets`. This particular implementation is very simplistic, meant more for demonstration than anything.
However, the server code which was used to build this access logging service can be found [here](https://github.com/solo-io/gloo/tree/master/projects/accesslogger/pkg/loggingservice) 
and is easily extendable to fit any needs. To run a different access logger than the one provided simply replace the image object in the helm configuration, and the service will be replaced by a custom image.
