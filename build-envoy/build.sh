# GLOODIR=$(dirname $PWD)
# docker run --rm -t -i -v $GLOODIR:$GLOODIR -w $GLOODIR/build-envoy envoyproxy/envoy-build-ubuntu bazel build -c dbg @envoy//source/exe:envoy-static
bazel build -c dbg //:envoy