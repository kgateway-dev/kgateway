FROM docker.io/library/golang:1.26.9-trixie
# Pinned Debian package versions break on every point release.
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git make jq \
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
RUN go build -o /usr/local/bin/kgateway ./cmd/kgateway \
    && go build -o /usr/local/bin/sds ./cmd/sds \
    && go build -o /usr/local/bin/envoyinit ./cmd/envoyinit
# Build the tests now so they run offline without recompiling. E2E tests (-tags e2e) need a kind cluster and are
# left out.
RUN go test -count=1 -run '^$' ./... \
    && (go test -timeout 35m ./... || echo "WARNING: go test reported failures")
