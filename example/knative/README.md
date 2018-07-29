In this document we will demo gloo together with knative. gloo version of 0.4.2 or latest is requried. This documented was test with knative serving v0.1.0.

# Install everything!

## Start Minikube
```
minikube start --memory=8192 --cpus=4   --kubernetes-version=v1.10.5   --vm-driver=kvm2   --bootstrapper=kubeadm   --extra-config=controller-manager.cluster-signing-cert-file="/var/lib/localkube/certs/ca.crt"   --extra-config=controller-manager.cluster-signing-key-file="/var/lib/localkube/certs/ca.key"   --extra-config=apiserver.admission-control="LimitRanger,NamespaceExists,NamespaceLifecycle,ResourceQuota,ServiceAccount,DefaultStorageClass,MutatingAdmissionWebhook"
```

## Install Gloo, Istio, Knative.

To install istio and knative, please run:

```
glooctl install
curl -L https://raw.githubusercontent.com/knative/serving/v0.1.0/third_party/istio-0.8.0/istio.yaml \
  | sed 's/LoadBalancer/NodePort/' \
  | kubectl apply -f -
  
curl -L https://github.com/knative/serving/releases/download/v0.1.0/release-lite.yaml \
  | sed 's/LoadBalancer/NodePort/' \
  | kubectl apply -f -

```

# Add a service
```
kubectl apply -f - <<EOF
apiVersion: serving.knative.dev/v1alpha1 # Current version of Knative
kind: Service
metadata:
  name: helloworld-go # The name of the app
  namespace: default
spec:
  runLatest:
    configuration:
      revisionTemplate:
        spec:
          container:
            image: gcr.io/knative-samples/helloworld-go # The URL to the image of the app
            env:
            - name: TARGET # The environment variable printed out by the sample app
              value: "Go Sample v1"
EOF
```

# Configure gloo
Verify and see that gloo detected the new upstream (may take a bit of time to show up untill all the pods start up):
```
glooctl upstream get
```

Add route to it!
```
glooctl route create --sort --path-prefix / --upstream helloworld-go.default.example.com
```

you can now use all gloo's features for your knative services!