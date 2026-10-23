FROM docker.io/library/golang:1.26.9-trixie
# Pinned Debian package versions break on every point release.
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git make jq clang libclang-dev \
    && rm -rf /var/lib/apt/lists/*
# The binary validator (strict Envoy config validation) shells out to /usr/local/bin/envoy. Match the ENVOY_IMAGE pin
# in the Makefile.
COPY --from=docker.io/envoyproxy/envoy:v1.39.4 /usr/local/bin/envoy /usr/local/bin/envoy
COPY . /src
WORKDIR /src
# Fetch every module the build, the tests and the tools/ module (setup-envtest, gotestsum) need.
RUN go mod download && go -C tools mod download
# envtest-backed tests download kube-apiserver and etcd at test time unless KUBEBUILDER_ASSETS is set; fetch them now,
# while online.
RUN ln -s "$(make -s --no-print-directory envtest-path)" /opt/envtest && ls /opt/envtest/
ENV KUBEBUILDER_ASSETS=/opt/envtest
# With no network there is no default route, and kube-apiserver will not start unless told its advertise address.
RUN mv /opt/envtest/kube-apiserver /opt/envtest/kube-apiserver.real \
    && printf '#!/bin/sh\nexec "$(dirname "$0")/kube-apiserver.real" --advertise-address=127.0.0.1 "$@"\n' \
      > /opt/envtest/kube-apiserver \
    && chmod +x /opt/envtest/kube-apiserver
# Tests that validate Envoy config shell out to `docker run envoyproxy/envoy`; run the local Envoy binary instead.
COPY .oss-scanner/docker-shim.sh /usr/local/bin/docker
# The Envoy dynamic module (librust_module.so, the rustformation and http-acl filters) is loaded from /usr/local/lib by
# the validator and by Envoy. rust-toolchain.toml pins the toolchain; install it now, while online.
ENV PATH=/root/.cargo/bin:$PATH CARGO_TERM_COLOR=never
RUN curl -fsSL https://sh.rustup.rs | sh -s -- -y --profile minimal --default-toolchain none \
    && cd internal/envoy_modules \
    && rustup toolchain install \
    && cargo fetch --locked \
    && cargo build --locked \
    && cp target/debug/librust_module.so /usr/local/lib/librust_module.so \
    && cargo test --workspace --locked --no-run
RUN go build -o /usr/local/bin/kgateway ./cmd/kgateway \
    && go build -o /usr/local/bin/sds ./cmd/sds \
    && go build -o /usr/local/bin/envoyinit ./cmd/envoyinit
# Build the tests now so they run offline without recompiling. E2E tests (-tags e2e) need a kind cluster and are
# left out.
RUN go test -count=1 -run '^$' ./... \
    && (go test -timeout 35m ./... || echo "WARNING: go test reported failures")
