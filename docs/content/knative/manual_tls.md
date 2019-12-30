---
title: Setting up Manual TLS
description: Manually configure server TLS for Knative services using Gloo.
---

At the time of writing, Knative only exposes TLS through cert-manager, in a feature called Automatic TLS (https://knative.dev/development/serving/using-auto-tls/). Automatic TLS works the same with Gloo and can be enabled by following the linked tutorial.

This guide shows how to directly configure Gloo to serve a Knative service on port 443 using server TLS. In this model, you specify the name of a TLS-cert secret (and optional SNI domains) on the *annotations* that live on your knative service.

It assumes you've already followed the [installation guide for Gloo and Knative]({{% versioned_link_path fromRoot="/installation/knative" %}}). 

### Before you start

### What you'll need
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- Kubernetes v1.14+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a great way to get a cluster up quickly.
- [Docker](https://www.docker.com) installed and running on your local machine, and a Docker Hub account configured (we'll use it for a container registry).

### Steps

1. First, [ensure Knative is installed with Gloo]({{% versioned_link_path fromRoot="/installation/knative" %}}). 

1. Next, let's create a private key and certificate to use for serving traffic. Uf you have your own key/cert pair, you can use those instead of creating self-signed certs here.

    ```bash
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
       -keyout tls.key -out tls.crt -subj "/CN=helloworld-go.default.example.com"
    ```
 
1. Now we should create the Kubernetes secret to hold this cert:

```bash
kubectl create secret tls my-knative-tls-secret \
      --key tls.key \
      --cert tls.crt \
      --namespace default
``` 
 
1. Next, create a `Knative Service` *with TLS annotations added to it*:

     For this demo, a simple helloworld application written in go will be used.
     Copy the YAML below to a file called `helloworld-go.yaml` and apply it with
     `kubectl`
  
      {{< highlight yaml "hl_lines=6-8" >}}
apiVersion: serving.knative.dev/v1alpha1
kind: Service
metadata:
  name: helloworld-go
  namespace: default
  annotations:
    gloo.networking.knative.dev/ssl.sni_domains: helloworld-go.default.example.com
    gloo.networking.knative.dev/ssl.secret_name: my-knative-tls-secret
spec:
  template:
    spec:
      containers:
        - image: gcr.io/knative-samples/helloworld-go
          env:
            - name: TARGET
              value: Go Sample v1
      {{< /highlight >}}
  
    ```
    kubectl apply -f helloworld-go.yaml
    ```

2. Send a request

     **Knative Services** are exposed via the *Host* header assigned by Knative. By
     default, Knative will use the header `Host`:
     `{service-name}.{namespace}.example.com`. You can discover the appropriate *Host* header by checking the URL Knative has assigned to the `helloworld-go` service created above.
  
     ```
     kubectl get ksvc helloworld-go -n default  --output=custom-columns=NAME:.metadata.name,URL:.status.url
     ```

     returns

     ```
     NAME            URL
     helloworld-go   http://helloworld-go.default.example.com
     ```
  
     Gloo will use the `Host` header to route requests to the correct
     service. You can send a request to the `helloworld-go` service with curl
     using the `Host` and URL of the Gloo Gateway from above:
  
     ```
     INGRESS_IP=$(kubectl get svc -n gloo-system knative-external-proxy  -o jsonpath='{.status.loadBalancer.ingress[*].ip}')
   
     curl -v -k \
        --resolve "helloworld-go.default.example.com:443:$INGRESS_IP" \
        https://helloworld-go.default.example.com:443
     ```

     returns

     ```
     * Added helloworld-go.default.example.com:443:34.74.251.190 to DNS cache
     * Rebuilt URL to: https://helloworld-go.default.example.com:443/
     * Hostname helloworld-go.default.example.com was found in DNS cache
     *   Trying 34.74.251.190...
     * TCP_NODELAY set
     * Connected to helloworld-go.default.example.com (34.74.251.190) port 443 (#0)
     * ALPN, offering h2
     * ALPN, offering http/1.1
     * Cipher selection: ALL:!EXPORT:!EXPORT40:!EXPORT56:!aNULL:!LOW:!RC4:@STRENGTH
     * successfully set certificate verify locations:
     *   CAfile: /etc/ssl/cert.pem
       CApath: none
     * TLSv1.2 (OUT), TLS handshake, Client hello (1):
     * TLSv1.2 (IN), TLS handshake, Server hello (2):
     * TLSv1.2 (IN), TLS handshake, Certificate (11):
     * TLSv1.2 (IN), TLS handshake, Server key exchange (12):
     * TLSv1.2 (IN), TLS handshake, Server finished (14):
     * TLSv1.2 (OUT), TLS handshake, Client key exchange (16):
     * TLSv1.2 (OUT), TLS change cipher, Client hello (1):
     * TLSv1.2 (OUT), TLS handshake, Finished (20):
     * TLSv1.2 (IN), TLS change cipher, Client hello (1):
     * TLSv1.2 (IN), TLS handshake, Finished (20):
     * SSL connection using TLSv1.2 / ECDHE-RSA-CHACHA20-POLY1305
     * ALPN, server accepted to use h2
     * Server certificate:
     *  subject: CN=helloworld-go.default.example.com
     *  start date: Dec 26 15:26:57 2019 GMT
     *  expire date: Dec 25 15:26:57 2020 GMT
     *  issuer: CN=helloworld-go.default.example.com
     *  SSL certificate verify result: self signed certificate (18), continuing anyway.
     * Using HTTP2, server supports multi-use
     * Connection state changed (HTTP/2 confirmed)
     * Copying HTTP/2 data in stream buffer to connection buffer after upgrade: len=0
     * Using Stream ID: 1 (easy handle 0x7fb9e7006600)
     > GET / HTTP/2
     > Host: helloworld-go.default.example.com
     > User-Agent: curl/7.54.0
     > Accept: */*
     >
     * Connection state changed (MAX_CONCURRENT_STREAMS updated)!
     < HTTP/2 200
     < content-length: 20
     < content-type: text/plain; charset=utf-8
     < date: Thu, 26 Dec 2019 15:29:12 GMT
     < x-envoy-upstream-service-time: 2
     < server: envoy
     < x-envoy-decorator-operation: activator-service.knative-serving.svc.cluster.local:80/*
     <
     Hello Go Sample v1!
     * Connection #0 to host helloworld-go.default.example.com left intact
     ```

Congratulations! We've just successfully connected to our Knative service over a secure HTTPS connection! Try out some of the more advanced tutorials for Knative in [the Knative documentation](https://knative.dev/docs/).
