FROM debian:bookworm-slim

# Install ca-certificates for HTTPS and create non-root user with home at /data
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    useradd -m -d /data -u 5454 thinkt

# Copy binary from goreleaser build
COPY thinkt /usr/local/bin/thinkt

USER thinkt
WORKDIR /data

ENTRYPOINT ["thinkt"]
