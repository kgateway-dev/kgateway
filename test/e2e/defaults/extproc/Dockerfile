FROM --platform=$BUILDPLATFORM golang:alpine AS builder

RUN apk --no-cache add make
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .

ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -a -o _output/server

FROM alpine:3.17.6
COPY --from=builder ./app/_output/server /server

USER 10101

EXPOSE 18080
CMD ["./server"]
