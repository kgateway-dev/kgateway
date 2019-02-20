# running locally on minikube

to update containers locally, you'll want to do the following:

1) `export GLOO_DIR=$GOPATH/src/github.com/solo-io/gloo`
2) run the `hack/kube/restart.sh <namespace>` script in this directory.
3) run `kubectl get pods -n <namespace>` and check that all pods are running

This script will rebuild all containers simultaneously and then create a 
new template with the tag to use all of the local images.
Very useful for locally testing with all changes.