#####################################################################################################################
#
#   This Dockerfile describes a container that serves as a reproducible build environment for gloo with race detection enabled.
#
#   Also, to be able to build with -race, the gloo image has to be built with CGO_ENABLED=1 (the Go runtime relies
#   on C code and libraries to load plugins), so cross compiling isn't as easy as `GOARCH=amd64 GOOS=linux go build`
#   anymore. Using this container you can build the binary on any architecture.
#
#####################################################################################################################
ARG GO_BUILD_IMAGE
FROM $GO_BUILD_IMAGE as build-env

ARG VERSION
ARG GCFLAGS
ARG LDFLAGS
ARG USE_APK
ARG GOARCH

# Fail if VERSION is not set
RUN if [[ ! $VERSION ]]; then echo "Required VERSION build argument not set" && exit 1; fi

RUN echo $USE_APK
RUN if [[ $USE_APK ]]; then apk add --no-cache gcc musl-dev git; fi

ADD . /go/src/github.com/solo-io/gloo
WORKDIR /go/src/github.com/solo-io/gloo

# Build gloo with CGO and -race enabled
RUN CGO_ENABLED=1 GOARCH=${GOARCH} GOOS=linux go build \
    -race \
    -gcflags="$GCFLAGS" \
    -ldflags="-X github.com/solo-io/gloo/pkg/version.Version=$VERSION $LDFLAGS" \
    -o gloo-linux-${GOARCH} \
    projects/gloo/cmd/main.go


FROM alpine:3.17.3

ARG GOARCH
COPY --from=build-env /go/src/github.com/solo-io/gloo/gloo-linux-${GOARCH} /
