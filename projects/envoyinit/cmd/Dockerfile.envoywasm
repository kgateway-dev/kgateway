FROM quay.io/solo-io/envoy-gloo-wasm:0.2.0

COPY envoyinit-linux-amd64 /usr/local/bin/envoyinit

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/envoyinit"]
CMD []