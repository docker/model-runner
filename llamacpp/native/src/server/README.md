This is a fork of the example inference server implemented as part of the
llama.cpp project. It has to be kept up to date with the vendored llama.cpp.
To do this, after bumping the llama.cpp submodule:

1. run `make` in `native/src/server`
2. if necessary resolve any conflicts manually and update the patch file
3. commit the changes (eventually we'll get rid of this step in favour of a
   fully automated workflow)

The primary objective of this fork is to quickly add any `llama-server` changes
required by the model runner and to maintain a minimal subset of non-upstreamable
changes. Currently we've upstreamed:

* unix socket support for mac and linux
* unix socket support for windows

We may want to upstream:

* making webui optional during compilation

Changes that we don't want to upstream:

* name change in headers returned by our `llama-server`
* support for reading the socket name from `DD_INF_UDS`
