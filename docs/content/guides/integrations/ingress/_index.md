---
title: Kubernetes Ingress 
weight: 5
description: Setting up Gloo Edge to handle Kubernetes Ingress Objects.
---

Kubernetes Ingress Controllers are for simple traffic routing in a Kubernetes cluster. Gloo Edge supports managing Ingress objects with the `glooctl install ingress` command. Gloo Edge will configure Envoy using [Kubernetes Ingress objects](https://kubernetes.io/docs/concepts/services-networking/ingress/) created by users. Learn more about [Kubernetes Ingress](https://www.solo.io/solutions/solutions-kubernetes-ingress/).

## Ingress Class

By default, Gloo Edge ignores the `kubernetes.io/ingress.class` [Ingress Class annotation](https://github.com/kubernetes/ingress-gce/blob/master/docs/faq/README.md#how-do-i-run-multiple-ingress-controllers-in-the-same-cluster) on Ingresses, meaning that Gloo Edge will enable routing for all detected Ingresses regardless of their ingress class.

To have Gloo Edge respect the Ingress Class annotation (Gloo Edge will only process Ingresses with the annotation `kubernetes.io/ingress.class: gloo`):

* Setting the `Values.ingress.requireIngressClass=true` in your Helm value overrides
* Directly setting the environment variable `REQUIRE_INGRESS_CLASS=true` on the `ingress` deployment


When Gloo Edge is set to require ingress class, the value `gloo` can be customized to match any arbitrary value by doing one of the following:

* Set the `Values.ingress.customIngressClass=VALUE` in your Helm value overrides
* Directly setting the environment variable `CUSTOM_INGRESS_CLASS=VALUE` on the `ingress` deployment.

This is useful when wishing to use multiple instances of the Gloo Edge ingress controller in the same Kubernetes cluster. 


If you need more advanced routing capabilities, we encourage you to use Gloo Edge `VirtualServices` by installing as `glooctl install gateway`. See the remaining routing documentation for more details on the extended capabilities Gloo Edge provides **without** needing to add lots of additional custom annotations to your Ingress Objects.

---

## What you'll need

* [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* Kubernetes v1.11.3+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a
great way to get a cluster up quickly.

---

## Basic Ingress Object managed by Gloo Edge

### Steps

1. The Gloo Edge Ingress [installed]({{% versioned_link_path fromRoot="/installation/ingress" %}}) and running on Kubernetes.

2. Next, deploy the Pet Store app to Kubernetes:

    ```shell
    kubectl apply \
      --filename https://raw.githubusercontent.com/solo-io/gloo/v1.11.x/example/petstore/petstore.yaml
    ```

3. Let's create a Kubernetes Ingress object to route requests to the petstore:

    ```yaml
    cat <<EOF | kubectl apply --filename -
    apiVersion: networking.k8s.io/v1
    kind: Ingress
    metadata:
     name: petstore-ingress
     annotations:
        # note: this annotation is only required if you've set 
        # REQUIRE_INGRESS_CLASS=true in the environment for 
        # the ingress deployment
        kubernetes.io/ingress.class: gloo
    spec:
      rules:
      - host: gloo.example.com
        http:
          paths:
          - path: /.*
            pathType: ImplementationSpecific
            backend:
              service:
                name: petstore
                port:
                  number: 8080
    EOF
    ```

We're specifying the host as `gloo.example.com` in this example. You should replace this with the domain for which you want to route traffic, or you may omit the host field to indicate all domains (`*`).

The domain will be used to match the `Host` header on incoming HTTP requests.


4. Validate Ingress routing looks to be set up and running.

    ```shell
    kubectl get ingress petstore-ingress
    ```

    ```noop
    NAME               HOSTS              ADDRESS   PORTS   AGE
    petstore-ingress   gloo.example.com             80      14h
    ```

5. Let's test the route `/api/pets` using `curl`. First, we'll need to get the address of Gloo Edge's Ingress proxy:


    ```shell
    INGRESS_URL=$(glooctl proxy url --name ingress-proxy)
    echo $INGRESS_URL
    ```

    ```shell
    http://35.238.21.0:80
    ```
    
6. Now we can access the petstore service through Gloo Edge:

    ```shell
    curl -H "Host: gloo.example.com" ${INGRESS_URL}/api/pets
    ```

    ```json
    [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
    ```
   
{{% notice note %}}
If you configure your DNS to resolve `gloo.example.com` to the Gloo Edge proxy URL (e.g. by modifying your `/etc/resolv.conf`), you can omit the `Host` header in the command above, and instead use the command:
       
```shell script
curl http://gloo.example.com/api/pets
```
    
{{% /notice %}}

---

## TLS Configuration

Now if you want to use TLS with an Ingress Object managed by Gloo Edge, here are the basic steps you need to follow.

1. You need to have a TLS key and certificate available as a Kubernetes secret. Let's create a self-signed one for our
example using `gloo.system.com` domain.

    ```shell
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout my_key.key -out my_cert.cert -subj "/CN=gloo.example.com/O=gloo.example.com"
    ```

    And then you need to create a tls secret in your Kubernetes cluster that your Ingress can reference

    ```shell
    kubectl create secret tls my-tls-secret --key my_key.key --cert my_cert.cert
    ```

1. If you want to add server-side TLS to your Ingress, you can add it as shown below. Note that it is important that the hostnames match in both the `tls` section and in the `rules` that you want to be covered by TLS.

    {{< highlight yaml "hl_lines=9-12 14" >}}
cat <<EOF | kubectl apply --filename -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: petstore-ingress
  annotations:
    kubernetes.io/ingress.class: gloo
spec:
  tls:
  - hosts:
    - gloo.example.com
    secretName: my-tls-secret
  rules:
  - host: gloo.example.com
    http:
      paths:
      - path: /.*
        pathType: ImplementationSpecific
        backend:
          service:
            name: petstore
            port:
              number: 8080
EOF
    {{< /highlight >}}


1. To access our service, we'll need to connect to the Gloo Edge Ingress's HTTPS port. Retrieve the HTTPS address like so:


    ```shell
    # get the IP:Port instead of the full URL this time
    INGRESS_HTTPS=$(glooctl proxy url --name ingress-proxy --port https | sed -n -e 's/^.*:\/\///p')
    echo $INGRESS_HTTPS
    ```

    ```shell
    35.238.21.0:443
    ```
        
1. Now we can access the petstore using end-to-end encryption like so:
    
    ```shell
    curl --cacert my_cert.cert --connect-to gloo.example.com:443:${INGRESS_HTTPS} https://gloo.example.com/api/pets
    ```

    ```json
    [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
    ```

---

## Next Steps

Great! Our ingress is up and running. Check out the [official docs](https://kubernetes.io/docs/concepts/services-networking/ingress) for more information on using Kubernetes Ingress Controllers.

For even greater routing capabilities, consider running [Gloo Edge in gateway mode]({{% versioned_link_path fromRoot="/guides/traffic_management/" %}}) alongside your Gloo Edge ingress installation. Simply set up one Gloo Edge instance in gateway mode and deploy another Gloo Edge instance that can process Kubernetes ingress objects. You can set up both Gloo Edge instances in the same cluster. 

