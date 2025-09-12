# opencode on Docker Model Runner

This directory defines a Docker image that can run opencode with local models
using Docker Model Runner. The opencode instances run inside containers,
providing additional strong isolation for side-effects that occur while
developing with opencode.

Also, because these models run locally in Docker Desktop, there are no API usage
limits or associated costs.


## Usage

This directory defines an Ubuntu-based base image called `opencode-dmr` that
includes the minimal setup for connecting opencode to Docker Model Runner. It
can be built using `./build.sh`, which will tag an image named
`opencode-dmr:latest`.

This image can be used directly, though it won't contain any toolchains other
than Node.js. The image expects the target working directory to be mounted at
`/code`, e.g.

```
# Run DMR-based opencode in a container on the current working directory.
docker run -it --rm -v .:/code opencode-dmr:latest

# Run DMR-based opencode on a different directory.
docker run -it --rm -v /some/path:/code opencode-dmr:latest
```

If you want to extend the image with additional toolchains (e.g. Go), you can
generate a new image, e.g.

```
# Start from the Ubuntu-based opencode image.
FROM opencode-dmr:latest

# Install and configure the toolchains we need.
RUN <<EOF
apt-get -y install golang
EOF
```

See `examples/opencode` for a concrete example of how this might look.


## Customization

TODO: Document customization flags.
