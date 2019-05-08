FROM golang:1.11.9-alpine3.9 AS build

ARG gcsfuse_version
ARG go_flags
ARG lint
ARG upx_flags

ENV DRIVER ${GOPATH}/src/driver
ENV GOFLAGS ${go_flags}
ENV LINT ${lint}
ENV UPX ${upx_flags}

RUN apk add --update --no-cache fuse fuse-dev git upx

# Build the driver
COPY driver ${DRIVER}
WORKDIR ${DRIVER}
RUN go get -u github.com/golang/dep/cmd/dep \
 && dep ensure -v
RUN go build -v -o /tmp/bin/driver .

WORKDIR ${GOPATH}

# Lint the driver
RUN if [ $LINT = "true" ]; then \
        apk add --update --no-cache gcc musl-dev \
     && wget -O - -q https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.16.0 \
     && golangci-lint run -E golint,gofmt --max-same-issues 0 ${DRIVER}; \
    fi

# Install gcsfuse using the specified version or commit hash
RUN go get -u github.com/googlecloudplatform/gcsfuse
RUN go install github.com/googlecloudplatform/gcsfuse/tools/build_gcsfuse
RUN mkdir /tmp/gcsfuse
RUN build_gcsfuse ${GOPATH}/src/github.com/googlecloudplatform/gcsfuse /tmp/gcsfuse ${gcsfuse_version} ${GOFLAGS}

# We don't need mount(8) compatibility as we call the binary directly, so only copy that
RUN mv /tmp/gcsfuse/bin/gcsfuse /tmp/bin/gcsfuse

# Compress the binaries
RUN upx /tmp/bin/driver
RUN upx /tmp/bin/gcsfuse

FROM alpine:3.9

# https://github.com/opencontainers/image-spec/blob/master/annotations.md
LABEL "org.opencontainers.image.authors"="Ofek Lev <ofekmeister@gmail.com>"
LABEL "org.opencontainers.image.description"="Docker Volume Plugin for Google Cloud Storage"
LABEL "org.opencontainers.image.licenses"="Apache-2.0 OR MIT"
LABEL "org.opencontainers.image.source"="https://github.com/ofek/docker-volume-gcs"
LABEL "org.opencontainers.image.title"="gcsfs"

RUN apk add --update --no-cache ca-certificates fuse && rm -rf /tmp/*

# Copy the binaries
COPY --from=build /tmp/bin/* /usr/local/bin/

# Allow non-root users to specify the allow_other or allow_root mount options
RUN echo "user_allow_other" > /etc/fuse.conf

# Create directories for mounts and temporary key storage
RUN mkdir -p /mnt/buckets /mnt/keys /tmp/keys

WORKDIR /

CMD ["/usr/local/bin/driver"]
