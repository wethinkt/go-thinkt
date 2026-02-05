# thinkt Dockerfile
# Copyright (c) 2025 Neomantra Corp / BrainSTM

ARG THINKT_BUILD_BASE="golang"
ARG THINKT_BUILD_TAG="1.25-bookworm"

ARG THINKT_RUNTIME_BASE="debian"
ARG THINKT_RUNTIME_TAG="bookworm-slim"

##############################################################################
# Builder
##############################################################################

FROM ${THINKT_BUILD_BASE}:${THINKT_BUILD_TAG} AS build

ARG TARGETARCH

ADD . /src
WORKDIR /src

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/thinkt ./cmd/thinkt

##############################################################################
# Runtime
##############################################################################

FROM ${THINKT_RUNTIME_BASE}:${THINKT_RUNTIME_TAG} AS runtime

ARG THINKT_BUILD_BASE="golang"
ARG THINKT_BUILD_TAG="1.25-bookworm"
ARG THINKT_RUNTIME_BASE="debian"
ARG THINKT_RUNTIME_TAG="bookworm-slim"
ARG TARGETARCH

# Install ca-certificates and create non-root user with home at /data
RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -d /data -u 5454 thinkt

# Copy binary from build stage
COPY --from=build /src/bin/thinkt /usr/local/bin/thinkt

USER thinkt
WORKDIR /data

# Labels
LABEL org.opencontainers.image.title="thinkt"
LABEL org.opencontainers.image.description="Tools for exploring LLM conversation traces"
LABEL THINKT_BUILD_BASE="${THINKT_BUILD_BASE}"
LABEL THINKT_BUILD_TAG="${THINKT_BUILD_TAG}"
LABEL THINKT_RUNTIME_BASE="${THINKT_RUNTIME_BASE}"
LABEL THINKT_RUNTIME_TAG="${THINKT_RUNTIME_TAG}"
LABEL THINKT_TARGET_ARCH="${TARGETARCH}"

ENTRYPOINT ["thinkt"]
