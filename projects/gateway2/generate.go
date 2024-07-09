package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen crd:maxDescLen=0 object paths="./api/..." output:crd:artifacts:config=../../install/helm/gloo/crds/

func main() {
	panic("this file is a go:generate template and should not be included in the final build")
}
