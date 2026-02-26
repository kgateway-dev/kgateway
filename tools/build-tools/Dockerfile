# Based on Istio's build-tools image: https://github.com/istio/tools/tree/228d21452fd640bf7389d7d41fecaa715ce73249/docker/build-tools
ARG GO_VERSION=1.25.7
ARG GO_BASE_IMAGE=golang:${GO_VERSION}-bookworm
ARG UBUNTU_BASE_IMAGE=ubuntu:noble

FROM ${GO_BASE_IMAGE} AS go_context
FROM ${UBUNTU_BASE_IMAGE} AS build_tools

ARG TARGETARCH
ARG VERSION=dev

LABEL "io.kgateway.repo"="https://github.com/kgateway-dev/kgateway"
LABEL "io.kgateway.version"="${VERSION}"

ENV DEBIAN_FRONTEND=noninteractive

# Pinned tool versions (keep aligned with `go.mod` and repo tooling where possible)
ENV RUST_VERSION=1.86.0
ENV KUBECTL_VERSION=1.35.0
ENV HELM_VERSION=v3.19.2
ENV PROTOC_VERSION=33.2
ENV BUF_VERSION=v1.62.1
ENV YQ_VERSION=4.50.1
ENV SU_EXEC_VERSION=0.3.1

# General env
ENV HOME=/home
ENV LANG=C.UTF-8
ENV CARGO_HOME=/home/.cargo
ENV RUSTUP_HOME=/home/.rustup

# Go env
ENV GO111MODULE=on
ENV GOTOOLCHAIN=local
ENV GOPROXY=https://proxy.golang.org
ENV GOSUMDB=sum.golang.org
ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV GOCACHE=/gocache
ENV GOBIN=/gobin
# Include /usr/sbin and /sbin so devcontainer features can find admin tools like groupadd.
ENV PATH=/usr/local/go/bin:/gobin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      wget \
      git \
      openssh-client \
      rsync \
      jq \
      unzip \
      xz-utils \
      file \
      make \
      gcc \
      g++ \
      libc-dev \
      pkg-config \
      passwd \
      python3 \
      python3-pip \
      python3-setuptools \
      python3-yaml \
      sudo \
      gnupg \
      lsb-release \
      vim \
    && rm -rf /var/lib/apt/lists/*

# Install Docker CLI + buildx plugin (Codespaces uses docker-outside-of-docker)
RUN --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    --mount=type=cache,target=/var/cache/apt,sharing=locked \
    install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=${TARGETARCH} signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
      docker-ce-cli \
      docker-buildx-plugin \
    && rm -rf /var/lib/apt/lists/*

# Go toolchain (copy from official golang image to match go.mod)
COPY --from=go_context /usr/local/go /usr/local/go

# Install protoc
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip" ;; \
      arm64) PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-aarch_64.zip" ;; \
      *) echo "unsupported TARGETARCH ${TARGETARCH}"; exit 1 ;; \
    esac; \
    wget -nv -O "/tmp/${PROTOC_ZIP}" "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"; \
    unzip -q "/tmp/${PROTOC_ZIP}" -d /tmp/protoc; \
    mv /tmp/protoc/bin/protoc /usr/local/bin/protoc; \
    chmod 555 /usr/local/bin/protoc; \
    rm -rf /tmp/protoc "/tmp/${PROTOC_ZIP}"

# kubectl
RUN set -eux; \
    curl -fsSL -o /usr/local/bin/kubectl "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl"; \
    chmod 555 /usr/local/bin/kubectl

# kind (pinned via go.mod tool dependency; wrapper script avoids version drift)
COPY tools/build-tools/kind /usr/local/bin/kind
RUN chmod 555 /usr/local/bin/kind

# helm
RUN set -eux; \
    curl -fsSL -o /tmp/helm.tgz "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz"; \
    mkdir -p /tmp/helm; \
    tar -xzf /tmp/helm.tgz -C /tmp/helm; \
    mv "/tmp/helm/linux-${TARGETARCH}/helm" /usr/local/bin/helm; \
    chmod 555 /usr/local/bin/helm; \
    rm -rf /tmp/helm /tmp/helm.tgz

# buf
RUN set -eux; \
    arch="$(uname -m)"; \
    curl -fsSL -o /usr/local/bin/buf "https://github.com/bufbuild/buf/releases/download/${BUF_VERSION}/buf-Linux-${arch}"; \
    chmod 555 /usr/local/bin/buf

# yq
RUN set -eux; \
    curl -fsSL -o /usr/local/bin/yq "https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_linux_${TARGETARCH}"; \
    chmod 555 /usr/local/bin/yq

# Rust toolchain (for internal/envoyinit)
RUN set -eux; \
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain "${RUST_VERSION}" --profile minimal --component rustfmt --component clippy; \
    ln -s "${CARGO_HOME}/bin/"* /usr/local/bin/

# Install su-exec (like Istio: small, reliable privilege drop for entrypoint)
RUN set -eux; \
    curl -fsSL -o "/tmp/su-exec.tgz" "https://github.com/NobodyXu/su-exec/archive/refs/tags/v${SU_EXEC_VERSION}.tar.gz"; \
    tar -xzf /tmp/su-exec.tgz -C /tmp; \
    pushd "/tmp/su-exec-${SU_EXEC_VERSION}" >/dev/null; \
      LDFLAGS="-fvisibility=hidden -Wl,-O2 -Wl,--discard-all -Wl,--strip-all -Wl,--as-needed -Wl,--gc-sections" make; \
      cp -a su-exec /usr/local/bin/su-exec; \
      chmod u+sx /usr/local/bin/su-exec; \
    popd >/dev/null; \
    rm -rf /tmp/su-exec.tgz "/tmp/su-exec-${SU_EXEC_VERSION}"

# Copy helper scripts from build context (shellcheck can lint these more easily than inlined scripts)
COPY tools/build-tools/docker-entrypoint.sh /usr/local/bin/docker-entrypoint
RUN chmod +x /usr/local/bin/docker-entrypoint

COPY tools/build-tools/bashrc /home/.bashrc

# Mountpoints for host mounts (Codespaces & local devcontainers)
RUN mkdir -p /go /gocache /gobin \
    && mkdir -p /config/.docker /config/.config/gcloud /config/.kube \
    && mkdir -p /config-copy \
    && mkdir -p /home/.cache /home/.cargo/registry /home/.cargo/git \
    && chmod -R 777 /go /gocache /gobin /config /config-copy /home/.cache /home/.cargo

WORKDIR /
ENTRYPOINT ["/usr/local/bin/docker-entrypoint"]
CMD ["bash"]


