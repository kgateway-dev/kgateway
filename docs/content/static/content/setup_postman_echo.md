This guide assumes that you installed the following components:
* Gloo Edge in the `gloo-system` namespace in your cluster
* The `glooctl` command line utility
* The [jq](https://stedolan.github.io/jq/) command line utility to format JSON strings

You also need an upstream service to serve as the target for the requests that you send to test the Gloo Edge configurations 
in this tutorial. You can use the publicly available [Postman Echo](https://postman-echo.com/) service. 
Postman Echo exposes a set of endpoints that are very useful for inspecting both the requests sent upstream and the resulting responses. For more information about this service, see the [Postman Echo documentation](https://docs.postman-echo.com/?version=latest).

Create a static upstream to represent the _postman-echo.com_ remote service.

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: postman-echo
  namespace: gloo-system
spec:
  static:
    hosts:
    - addr: postman-echo.com
      port: 80
```