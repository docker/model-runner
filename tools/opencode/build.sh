#!/bin/bash

docker build -t opencode-dmr:core --target core .
docker build -t opencode-dmr:extended --target extended .
