package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen crd object paths="./api/..." output:crd:artifacts:config=../../install/helm/gloo/crds/
//go:generate go run github.com/ahmetb/gen-crd-api-reference-docs -template-dir docs/conf -config docs/conf/config.json -api-dir=github.com/solo-io/gloo/projects/gateway2/api/ -out-file docs/docs.html
