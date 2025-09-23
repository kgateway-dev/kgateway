ARG ENVOY_IMAGE



FROM ghcr.io/rust-cross/cargo-zigbuild:0.19.88@sha256:a00024c136a365171892a186b1ee5552812a8f91f2ba9f5fe609c0d352238cca AS rust_builder

WORKDIR /build

RUN apt update && apt install -y clang



# Then copy the rest of the source code and build the library.
# Paramerized to allow for supporting local, make and goreleaser builds
ARG RUSTFORMATIONS_DIR=./rustformations
COPY ${RUSTFORMATIONS_DIR}/Cargo.toml ${RUSTFORMATIONS_DIR}/Cargo.lock ./
RUN mkdir src && echo "" > src/lib.rs
RUN cargo fetch
RUN rm -rf src

# Then copy the rest of the source code and build the library.
COPY ./${RUSTFORMATIONS_DIR} .
RUN cargo zigbuild --target x86_64-unknown-linux-gnu

# hard code amd64 for now as we only have that version of envoy-gloo
RUN cp /build/target/x86_64-unknown-linux-gnu/debug/librust_module.so /build/amd64_librust_module.so

FROM $ENVOY_IMAGE AS envoy
# hard code amd64 for now as we only have that version of envoy-gloo
ARG GOARCH=amd64
# eventually may matter for now https://unix.stackexchange.com/a/701288
# means its not too useful
ENV DEBIAN_FRONTEND=noninteractive

# Update our deps to make cve toil lower
#install wget for our default probes
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install wget ca-certificates -y \
    && rm -rf  /var/log/*log /var/lib/apt/lists/* /var/log/apt/* /var/lib/dpkg/*-old /var/cache/debconf/*-old

COPY envoyinit-linux-$GOARCH /usr/local/bin/envoyinit

ENV ENVOY_DYNAMIC_MODULES_SEARCH_PATH=/usr/local/lib
COPY --from=rust_builder /build/amd64_librust_module.so /usr/local/lib/librust_module.so


# SDS-specific setup, only used if ENVOY_SIDECAR=true
ARG ENTRYPOINT_SCRIPT=/docker-entrypoint.sh
COPY $ENTRYPOINT_SCRIPT /

USER 10101

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD []
