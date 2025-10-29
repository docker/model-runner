FROM scratch AS suffix-windows
ENV BINARY_SUFFIX=".exe"

FROM scratch AS suffix-darwin
ENV BINARY_SUFFIX=""

FROM scratch AS suffix-linux
ENV BINARY_SUFFIX=""

FROM suffix-$TARGETOS
ARG TARGETARCH
ARG TARGETOS
COPY --from=dist dist/$TARGETOS-$TARGETARCH/docker-model$BINARY_SUFFIX /cli-plugins/model/$TARGETOS/docker-model$BINARY_SUFFIX
COPY --from=desktop-module module-metadata.json /