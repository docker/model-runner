# Use a recent Go base image to build the entrypoint. We'll use something based
# on Debian to ensure glibc compatibility with our final Ubuntu-baed base image,
# even though we disable cgo (so we could probably use Alpine too).
FROM golang:1.25-bookworm AS epbuilder

# Copy and build the entrypoint. Disable cgo since we don't need it.
ENV CGO_ENABLED=0
COPY ["entrypoint.go", "/root/entrypoint.go"]
RUN ["go", "build", "-o", "/root/entrypoint", "/root/entrypoint.go"]


# Use a recent Ubuntu base image.
FROM ubuntu:25.04 AS core

# Install the Docker CLI with the Docker Model Runner plugin.
RUN <<EOF
apt-get update
apt-get -y install ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" | \
  tee /etc/apt/sources.list.d/docker.list > /dev/null
apt-get update
apt-get -y install docker-ce-cli docker-model-plugin
EOF

# Install opencode.
RUN <<EOF
apt-get update
apt-get -y install nodejs npm
npm install -g opencode-ai
EOF

# Allocate storage for the opencode configuration and set up the authentication
# configuration for Docker Model Runner.
RUN <<EOF
mkdir -p /root/.local/share/opencode
mkdir -p /root/.config/opencode
EOF

# Copy the entrypoint binary from its builder.
COPY --from=epbuilder --chown=root:root --chmod=0700 ["/root/entrypoint", "/root/entrypoint"]

# Set the entrypoint.
ENTRYPOINT ["/root/entrypoint"]

# Create an extended build stage with additional tools.
FROM core AS extended

# Install additional development tools.
RUN <<EOF
apt-get -y install git build-essential gdb cmake golang delve rust-all jq
npm install -g corepack
EOF
