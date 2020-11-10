This guide assumes that you have installed Gloo Edge into the `gloo-system` namespace and that `glooctl` is installed on your
machine. We will also use the [jq](https://stedolan.github.io/jq/) command line utility to pretty print JSON strings.

We will need an upstream service to serve as the target for the requests that we will send to test the Gloo Edge configurations 
in this tutorial. To this end, we will use the publicly available [Postman Echo](https://postman-echo.com/) service. 
It exposes a set of endpoints that are very useful for inspecting both the requests sent upstream and the resulting responses; 
please refer to the [official documentation](https://docs.postman-echo.com/?version=latest) for more information about the service.

Let's create a static upstream to represent the _postman-echo.com_ remote service.

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