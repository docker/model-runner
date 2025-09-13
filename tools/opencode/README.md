# opencode on Docker Model Runner

This directory defines a Docker image that can run
[opencode](https://opencode.ai/) with local models using
[Docker Model Runner](https://docs.docker.com/ai/model-runner/). The opencode
instances run inside containers, providing additional strong isolation for
side-effects that occur while developing with opencode.

Because these models run locally in Docker Desktop, there are no API usage
limits or associated costs — only your own hardware is used.


## Usage

This directory defines an Ubuntu-based base image called `opencode-dmr:core`
that includes the minimal setup for connecting opencode to Docker Model Runner.
The image can be built using `./build.sh`. The `opencode-dmr:core` image can be
used directly, though it won't contain any toolchains other than Node.js. An
additional image is also built, `opencode-dmr:extended`, which includes common
development tools.

Both images expect the target working directory to be mounted at `/code`, e.g.

```
# Run DMR-based opencode in a container on the current working directory.
docker run -it --rm -v .:/code opencode-dmr:extended

# Run DMR-based opencode on a different directory.
docker run -it --rm -v /some/path:/code opencode-dmr:extended
```

If you want to extend the image with additional toolchains (e.g. Go), you can
generate a new image, e.g.

```
# Start from the minimal Ubuntu-based opencode image.
FROM opencode-dmr:core

# Install and configure the toolchains we need.
RUN <<EOF
apt-get -y install golang
EOF
```

See `examples/opencode` for a concrete example of how this might look.


## Customization

The `opencode-dmr` images come with predefined size profiles that will use
models compatible with your system's VRAM capacity. The current profiles
include:

- `medium`: For systems with 32 GB of VRAM or more (the default)
- `large`: For systems with 64 GB of VRAM or more
- `xl`: For systems with 128 GB of VRAM or more

These sizes are just estimates — your mileage may vary.

You can specify the size profile to use when running the image, e.g.

```
docker run -it --rm -v .:/code opencode-dmr:extended -s xl
```

For a complete list of override flags supported by the image, see:

```
docker run -it --rm -v .:/code opencode-dmr:extended -h
```

You'll find the profiles defined in `entrypoint.go`.
