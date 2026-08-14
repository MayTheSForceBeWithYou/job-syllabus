# Dedicated, minimal Fargate agent for exactly one job: build+push the
# api image with Kaniko. Split out of ci/agent.Dockerfile after Kaniko
# repeatedly killed the JNLP agent connection mid-build on that image —
# root cause: Kaniko has no daemon/overlayfs isolation, so it unpacks the
# Dockerfile's base image rootfs (golang:1.26-bookworm) directly onto its
# own container's filesystem, which on the general-purpose agent image
# means on top of terraform/node/python/trivy/checkov/tfsec/golangci-lint
# and everything else — real risk of corrupting shared system paths (both
# are Debian-family images with heavily overlapping paths like /usr/bin,
# /lib) out from under the very JVM process trying to keep the build
# alive. This image has nothing else running or installed to corrupt.
FROM gcr.io/kaniko-project/executor:latest AS kaniko

FROM jenkins/inbound-agent:latest-jdk21

USER root

COPY --from=kaniko /kaniko/executor /kaniko/executor

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl unzip \
    && curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o /tmp/awscliv2.zip \
    && unzip -q /tmp/awscliv2.zip -d /tmp \
    && /tmp/aws/install \
    && rm -rf /tmp/awscliv2.zip /tmp/aws /var/lib/apt/lists/*

# Deliberately no `USER jenkins` — same reasoning as ci/agent.Dockerfile:
# Kaniko isn't designed to run non-root.
