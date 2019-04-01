FROM soloio/envoy-gloo:0.1.4

COPY envoyinit-linux-amd64 /usr/local/bin/envoyinit

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/envoyinit"]
CMD []