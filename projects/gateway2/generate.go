package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen crd object paths="./api/..." output:crd:artifacts:config=../../install/helm/gloo/crds/
