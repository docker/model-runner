#!/bin/bash

# Ensure our opencode environment is built.
docker build -t custom-opencode:latest .

# Run our custom opencode environment on the current directory.
docker run --rm -it -v .:/code custom-opencode:latest
