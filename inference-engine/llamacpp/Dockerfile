# syntax=docker/dockerfile:1

FROM scratch
ARG TARGETOS
ARG TARGETARCH
ARG ACCEL
COPY --from=release-artifacts /com.docker.llama-server.native.$TARGETOS.$ACCEL.$TARGETARCH /com.docker.llama-server.native.$TARGETOS.$ACCEL.$TARGETARCH
