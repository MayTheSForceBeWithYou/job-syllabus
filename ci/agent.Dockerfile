# Ephemeral Fargate build agent — docs/design.md §10. Zero idle cost, clean
# workspace per build, no snowflake agent drift. Built on the official
# inbound-agent image so it speaks the Amazon ECS plugin's expected
# JNLP/WebSocket protocol out of the box; everything below is the tooling
# api-build/infra-plan/infra-apply/client-build actually need.
#
# Docker-in-Docker on Fargate is painful (§10) — this installs the Docker
# CLI + buildx only, for use against a remote BuildKit builder, not a local
# privileged daemon.
FROM jenkins/inbound-agent:latest-jdk21

USER root

# Matches go.mod's declared version (and the local dev toolchain) — a
# mismatch here is what made golangci-lint auto-fetch an intermediate
# Go toolchain (1.25) to build itself against, then refuse to lint a
# go.mod targeting a newer language version than it was built with.
ARG GO_VERSION=1.26.5
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV CGO_ENABLED=1

# `go test -race` (api-build's Test stage) needs CGO, which needs a real C
# toolchain — without this, the very first real CI run fails on that stage.
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# golangci-lint — api-build's Lint stage. Confirmed missing against a
# real CI run: "golangci-lint: not found", exit 127. `go install`, not
# the project's own shell installer: that script's embedded checksum
# didn't match its own downloaded binary against a real build (likely a
# stale/inconsistent CDN artifact) — `go install` sidesteps it entirely
# since Go's toolchain is already set up in this image.
# Pinned to match the version used locally (`golangci-lint --version`) —
# same config schema, same lint results either place.
RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 \
    && mv "$(go env GOPATH)/bin/golangci-lint" /usr/local/bin/

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl gnupg lsb-release unzip jq \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian $(lsb_release -cs) stable" \
        > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends docker-ce-cli docker-buildx-plugin \
    && rm -rf /var/lib/apt/lists/*

ARG TERRAFORM_VERSION=1.9.8
RUN curl -fsSL "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip" -o /tmp/tf.zip \
    && unzip -q /tmp/tf.zip -d /usr/local/bin \
    && rm /tmp/tf.zip

# Node + eas-cli — for client-build (Phase 6+); harmless to have early.
ARG NODE_MAJOR=20
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install -g eas-cli \
    && rm -rf /var/lib/apt/lists/*

# Trivy — fail-on-HIGH image scanning gate in api-build (§10/§11).
RUN curl -fsSL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh \
        | sh -s -- -b /usr/local/bin

# tfsec + checkov — infra-plan's security-scanning gates (§10). The
# install script uses bash-only syntax (array += appends) that silently
# breaks under `sh` (dash on this base image) — confirmed against a real
# build, which failed with "File to download not supplied" instead of an
# obvious syntax error.
RUN curl -fsSL https://raw.githubusercontent.com/aquasecurity/tfsec/master/scripts/install_linux.sh \
        | bash
# --ignore-installed is load-bearing: pip otherwise tries to uninstall
# apt-managed packages (e.g. "packaging") that have no RECORD file for it
# to work with, and fails outright — confirmed against a real build.
RUN apt-get update && apt-get install -y --no-install-recommends python3-pip \
    && pip3 install --no-cache-dir --break-system-packages --ignore-installed checkov \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o /tmp/awscliv2.zip \
    && unzip -q /tmp/awscliv2.zip -d /tmp \
    && /tmp/aws/install \
    && rm -rf /tmp/awscliv2.zip /tmp/aws

USER jenkins
