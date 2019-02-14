---
title: Gloo with Knative
weight: 5
---

Google's Knative project leverages a Kubernetes Cluster Ingress Controller to route requests to apps managed and autoscaled by Knative.
At the time of writing, the only available options for Cluster Ingress are Istio and Gloo. This tutorial explains how to get started 
using Gloo as your Knative Cluster Ingress.

### What you'll need
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- Kubernetes v1.11.3+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a great way to get a cluster up quickly.
- [Docker](https://www.docker.com) installed and running on your local machine, and a Docker Hub account configured (we'll use it for a container registry).

### Steps

1. First, deploy the Gloo Cluster Ingress and Knative [installed](../../installation) on Kubernetes. Knative can be deployed 
independently, or using `glooctl install knative`.
 
 
1. Next, we'll create a sample Go web app to deploy with Knative. 

    - Create a new file named `helloworld.go` and paste the following code. This code creates a basic web server which listens on port 8080:
              
            package main
            
            import (
                "fmt"
                "log"
                "net/http"
                "os"
            )
            
            func handler(w http.ResponseWriter, r *http.Request) {
                log.Print("Hello world received a request.")
                target := os.Getenv("TARGET")
                if target == "" {
                    target = "World"
                }
                fmt.Fprintf(w, "Hello %s!\n", target)
            }
            
            func main() {
                log.Print("Hello world sample started.")
            
                http.HandleFunc("/", handler)
            
                port := os.Getenv("PORT")
                if port == "" {
                    port = "8080"
                }
            
                log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
            }
      
    - in your project directory, create a file named `Dockerfile` and copy the code
      block below into it. For detailed instructions on dockerizing a Go app, see
      [Deploying Go servers with Docker](https://blog.golang.org/docker).
      
            # Use the official Golang image to create a build artifact.
            # This is based on Debian and sets the GOPATH to /go.
            # https://hub.docker.com/_/golang
            FROM golang as builder
            
            # Copy local code to the container image.
            WORKDIR /go/src/github.com/knative/docs/helloworld
            COPY . .
            
            # Build the helloworld command inside the container.
            # (You may fetch or manage dependencies here,
            # either manually or with a tool like "godep".)
            RUN CGO_ENABLED=0 GOOS=linux go build -v -o helloworld
            
            # Use a Docker multi-stage build to create a lean production image.
            # https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
            FROM alpine
            
            # Copy the binary to the production image from the builder stage.
            COPY --from=builder /go/src/github.com/knative/docs/helloworld/helloworld /helloworld
            
            # Service must listen to $PORT environment variable.
            # This default value facilitates local development.
            ENV PORT 8080
            
            # Run the web service on container startup.
            CMD ["/helloworld"]

    - Create a new file, `service.yaml` and copy the following service definition
     into the file. Make sure to replace `{username}` with your Docker Hub
     username.
      
            apiVersion: serving.knative.dev/v1alpha1
            kind: Service
            metadata:
              name: helloworld-go
              namespace: default
            spec:
              runLatest:
                configuration:
                  revisionTemplate:
                    spec:
                      container:
                        image: docker.io/{username}/helloworld-go
                        env:
                          - name: TARGET
                            value: "Go Sample v1"

1. Once the sample code has been created, we'll build and deploy it

    - Use Docker to build the sample code into a container. To build and push with
    Docker Hub, run these commands replacing `{username}` with your Docker Hub
    username:
    
            # Build the container on your local machine
            docker build -t {username}/helloworld-go .
            
            # Push the container to docker registry
            docker push {username}/helloworld-go
        

    - After the build has completed and the container is pushed to docker hub, you
    can deploy the app into your cluster. Ensure that the container image value
    in `service.yaml` matches the container you built in the previous step. Apply
    the configuration using `kubectl`:
    
            kubectl apply --filename service.yaml

    - Now that your service is created, Knative will perform the following steps:
          - Create a new immutable revision for this version of the app.
          - Network programming to create a route, ingress, service, and load balance
          for your app.
          - Automatically scale your pods up and down (including to zero active pods).
 
    - Run the following command to find the external IP address for the Gloo cluster ingress.
             
            CLUSTERINGRESS_URL=$(glooctl proxy url --name clusteringress-proxy)
            echo $CLUSTERINGRESS_URL
            http://192.168.99.230:31864
         
    - Run the following command to find the domain URL for your service:
      
            kubectl get ksvc helloworld-go -n default  --output=custom-columns=NAME:.metadata.name,DOMAIN:.status.domain
      
         Example:
      
            NAME                DOMAIN
            helloworld-go       helloworld-go.default.example.com
 
    - Test your app by sending it a request. Use the following `curl` command with
         the domain URL `helloworld-go.default.example.com` and `EXTERNAL-IP` address
         that you retrieved in the previous steps:
      
            curl -H "Host: helloworld-go.default.example.com" ${CLUSTERINGRESS_URL}
            Hello Go Sample v1!
            
         > Note: Add `-v` option to get more detail if the `curl` command failed.
      
    - Removing the sample app deployment      
      To remove the sample app from your cluster, delete the service record:
      
            kubectl delete --filename service.yaml


Great! our Knative ingress is up and running. See https://github.com/knative/docs for more information on using Knative.
