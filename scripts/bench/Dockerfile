# Reproducible bench environment.
# Build:  docker build -f scripts/bench/Dockerfile -t unitpm-bench .
# Run:    docker run --rm unitpm-bench
#
# All third-party fetches are pinned by digest/hash. Refresh = one PR.

FROM ubuntu:24.04@sha256:c4a8d5503dfb2a3eb8ab5f807da5bc69a85730fb49b5cfca2330194ebcc41c7b

ARG GO_VERSION=1.26.4
ARG NODE_VERSION=24.15.0
ARG NODE_SHA256=472655581fb851559730c48763e0c9d3bc25975c59d518003fc0849d3e4ba0f6
# pm2 version + per-dep sha512 integrity lives in scripts/bench/pm2/package-lock.json.
# supervisor version + sha256 lives in scripts/bench/requirements.txt.

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
		ca-certificates \
		curl \
		jq \
		git \
		xz-utils \
		openssl \
		build-essential \
		python3 \
		python3-pip \
		procps \
	&& rm -rf /var/lib/apt/lists/*

# Go (pinned by version; go.dev/dl serves immutable tarballs).
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
	| tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:/usr/local/bin:$PATH

# Node + npm (pinned by sha256 from nodejs.org SHASUMS256.txt).
RUN curl -fsSLO "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
	&& echo "${NODE_SHA256}  node-v${NODE_VERSION}-linux-x64.tar.xz" | sha256sum -c - \
	&& tar -xJf "node-v${NODE_VERSION}-linux-x64.tar.xz" -C /usr/local --strip-components=1 \
	&& rm "node-v${NODE_VERSION}-linux-x64.tar.xz"

# pm2 (pinned via npm ci against committed package-lock.json — every dep
# gets verified against its sha512 integrity from the lockfile).
COPY scripts/bench/pm2 /opt/pm2
RUN cd /opt/pm2 \
	&& npm ci --omit=optional \
	&& ln -s /opt/pm2/node_modules/.bin/pm2 /usr/local/bin/pm2

# supervisord (pinned by sha256 via --require-hashes).
COPY scripts/bench/requirements.txt /tmp/bench-requirements.txt
RUN pip install --no-cache-dir --break-system-packages --require-hashes \
		-r /tmp/bench-requirements.txt \
	&& rm /tmp/bench-requirements.txt

WORKDIR /src
COPY . /src

RUN go build -ldflags='-s -w' -o bin/unitpmd ./cmd/unitpmd \
	&& go build -ldflags='-s -w' -o bin/unitpm ./cmd/unitpm

CMD ["bash", "scripts/bench/run.sh"]
