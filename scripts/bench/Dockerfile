# Reproducible bench environment.
# Build:  docker build -f scripts/bench/Dockerfile -t lynx-bench .
# Run:    docker run --rm lynx-bench
#
# Pinned tool versions live as build args so a refresh is one PR.

FROM ubuntu:24.04

ARG GO_VERSION=1.26.2
ARG NODE_VERSION=24
ARG PM2_VERSION=6.0.14
ARG SUPERVISOR_VERSION=4.3.0

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
		ca-certificates \
		curl \
		jq \
		git \
		build-essential \
		python3 \
		python3-pip \
		procps \
	&& rm -rf /var/lib/apt/lists/*

# Go (pinned).
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
	| tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH

# Node + pm2 (pinned).
RUN curl -fsSL "https://deb.nodesource.com/setup_${NODE_VERSION}.x" | bash - \
	&& apt-get install -y --no-install-recommends nodejs \
	&& rm -rf /var/lib/apt/lists/* \
	&& npm install -g "pm2@${PM2_VERSION}"

# supervisord (pinned).
RUN pip install --no-cache-dir --break-system-packages "supervisor==${SUPERVISOR_VERSION}"

WORKDIR /src
COPY . /src

RUN go build -ldflags='-s -w' -o bin/lynxd ./cmd/lynxd \
	&& go build -ldflags='-s -w' -o bin/lynxpm ./cmd/lynxpm

CMD ["bash", "scripts/bench/run.sh"]
