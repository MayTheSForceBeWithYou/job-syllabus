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

ARG GO_VERSION=1.23.4
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"
ENV CGO_ENABLED=1

# `go test -race` (api-build's Test stage) needs CGO, which needs a real C
# toolchain — without this, the very first real CI run fails on that stage.
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev \
    && rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl gnupg lsb-release unzip \
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

# tfsec + checkov — infra-plan's security-scanning gates (§10).
RUN curl -fsSL https://raw.githubusercontent.com/aquasecurity/tfsec/master/scripts/install_linux.sh \
        | sh
RUN apt-get update && apt-get install -y --no-install-recommends python3-pip \
    && pip3 install --no-cache-dir --break-system-packages checkov \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o /tmp/awscliv2.zip \
    && unzip -q /tmp/awscliv2.zip -d /tmp \
    && /tmp/aws/install \
    && rm -rf /tmp/awscliv2.zip /tmp/aws

USER jenkins
