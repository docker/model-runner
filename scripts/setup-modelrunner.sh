#!/bin/bash
set -e

# Create modelrunner user if it doesn't exist
if ! id -u modelrunner >/dev/null 2>&1; then
    groupadd --system modelrunner
    # Add to video group for GPU access (if it exists)
    # TODO: if the render group ever gets a fixed GID add modelrunner to it
    if getent group video >/dev/null 2>&1; then
        useradd --system --gid modelrunner -G video --create-home --home-dir /home/modelrunner modelrunner
    else
        useradd --system --gid modelrunner --create-home --home-dir /home/modelrunner modelrunner
    fi
fi

# Create directories and set proper permissions
mkdir -p /var/run/model-runner /app/bin /models
chown -R modelrunner:modelrunner /var/run/model-runner /app /models
chmod -R 755 /models
