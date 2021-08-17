FROM golang:alpine AS builder

RUN apk --no-cache add make
WORKDIR /app
COPY . .

FROM alpine
COPY ./server ./server
CMD ["./server"]
