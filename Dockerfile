FROM alpine:3.21

# Install runtime dependencies required by the CLI
RUN apk add --no-cache \
    git \
    helm \
    sops \
    age \
    kubectl \
    bash \
    ca-certificates

WORKDIR /work

# GoReleaser will copy the binary to the root of the build context
COPY cluster-bootstrap-cli /usr/local/bin/cluster-bootstrap-cli

# Set the entrypoint to the absolute path
ENTRYPOINT ["/usr/local/bin/cluster-bootstrap-cli"]
