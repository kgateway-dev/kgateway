 clone http://github.com/googleapis/googleapis and set GOOGLE_PROTOS_HOME to its local location
 clone https://github.com/protocolbuffers/protobuf and set PROTOBUF_HOME to the location of its src folder.

# example:
```
cd /tmp/
git clone https://github.com/protocolbuffers/protobuf
git clone http://github.com/googleapis/googleapis
export PROTOBUF_HOME=$PWD/protobuf/src
export GOOGLE_PROTOS_HOME=$PWD/googleapis
cd -
go generate 
 ```

Test with gloo:
```
kubectl apply -f https://github.com/solo-io/gloo/releases/download/v0.13.16/gloo-gateway.yaml
kubectl apply -f bookstore.yaml
glooctl add route --name default --namespace gloo-system --path-prefix / --dest-name default-bookstore-8080 --dest-namespace gloo-system
ADDRESS=$(glooctl proxy address)
grpcurl -plaintext $ADDRESS main.Bookstore.ListShelves
```