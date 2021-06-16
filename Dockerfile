ARG GO_VERSION=1.16
ARG GO_IMAGE=golang:${GO_VERSION}

FROM --platform=${BUILDPLATFORM} $GO_IMAGE AS build
SHELL ["/bin/bash", "-xec"]
WORKDIR /root/build
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY . /root/build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH
RUN \
	if [ "$GOARCH" = "arm" ]; then \
		export GOARM="${TARGETVARIANT//v}"; \
	fi; \
	go build

FROM scratch
COPY --from=build /root/build/gh-artifacts-downloader /