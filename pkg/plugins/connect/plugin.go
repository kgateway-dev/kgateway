package connect

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/lyft/protoc-gen-validate/ --gogo_out=${GOPATH}/src/ envoy/api/envoy/config/filter/network/client_certificate_restriction/v2/client_certificate_restriction.proto
