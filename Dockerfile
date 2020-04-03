FROM golang:1.13-alpine as builder

RUN apk add busybox-static

WORKDIR /go/src/github.com/kelda-inc/blimp

ADD ./go.mod ./go.mod
ADD ./go.sum ./go.sum
ADD ./pkg ./pkg
ADD ./vendor ./vendor

ARG COMPILE_FLAGS

# Build and install one directory at a time, so that we get more or less decent
# caching.

ADD ./cli ./cli
RUN CGO_ENABLED=0 go install -mod=vendor -ldflags "${COMPILE_FLAGS}" ./cli/...

ADD ./registry ./registry
RUN CGO_ENABLED=0 go install -mod=vendor -ldflags "${COMPILE_FLAGS}" ./registry/...

ADD ./sandbox ./sandbox
RUN CGO_ENABLED=0 go install -mod=vendor -ldflags "${COMPILE_FLAGS}" ./sandbox/...

ADD ./cluster-controller ./cluster-controller
RUN CGO_ENABLED=0 go install -mod=vendor -ldflags "${COMPILE_FLAGS}" ./cluster-controller/...

ADD . .

RUN CGO_ENABLED=0 go install -mod=vendor -ldflags "${COMPILE_FLAGS}" ./...

RUN mkdir /gobin
RUN cp /go/bin/cluster-controller /gobin/blimp-cluster-controller
RUN cp /go/bin/syncthing /gobin/blimp-syncthing
RUN cp /go/bin/init /gobin/blimp-init
RUN cp /go/bin/sbctl /gobin/blimp-sbctl
RUN cp /go/bin/registry /gobin/blimp-auth
RUN cp /go/bin/vcp /gobin/blimp-vcp

FROM alpine

COPY --from=builder /bin/busybox.static /bin/busybox.static
COPY --from=builder /gobin/* /bin/
