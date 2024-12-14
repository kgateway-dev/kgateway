#!/bin/bash

set -e
set -x

go run sigs.k8s.io/controller-tools/cmd/controller-gen crd:maxDescLen=0 object paths="./api/..." output:crd:artifacts:config=../../install/helm/gloo/crds/
go run k8s.io/code-generator/cmd/register-gen --output-file zz_generated.register.go "./api/v1alpha1"