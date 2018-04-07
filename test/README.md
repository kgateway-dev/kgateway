Testing 
----

This repository contains end-to-end tests, as well as common test helpers for various Gloo tests.

To use debug binaries for tests use `DEBUG_IMAGES=1`. This uses `go build -i` which should result 
in a faster build.

To run kubernetes e2e use `RUN_KUBE_TESTS=1`. The currently configured cluster with kubectl will be 
used. If minikube is configured, docker images will not be pushed to the cloud, which may save time.

To use both options, run like so:
```
$ RUN_KUBE_TESTS=1 DEBUG_IMAGES=1 ginkgo -r
```