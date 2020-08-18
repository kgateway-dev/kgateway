 clone http://github.com/googleapis/googleapis and set GOOGLE_PROTOS_HOME to its local location
 clone https://github.com/protocolbuffers/protobuf and set PROTOBUF_HOME to the location of its src folder.

# regenerate descriptors:
```
cd /tmp/
git clone https://github.com/protocolbuffers/protobuf
git clone http://github.com/googleapis/googleapis
export PROTOBUF_HOME=$PWD/protobuf/src
export GOOGLE_PROTOS_HOME=$PWD/googleapis
cd -
go generate
 ```