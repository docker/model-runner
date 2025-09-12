# opencode Toolchain Example using Docker Model Runner

This example uses `tools/opencode` to demonstrate using opencode backed by
Docker Model Runner with a Go toolchain.


## Usage

First ensure that `opencode-dmr:latest` is available by running `./build.sh`
in the `tools/opencode` directory.

Then, to start the opencode environment, run `./run.sh`.

From there, you can use the standard opencode UI.

Some sample prompts to try:

```
I'd like you to write me a Fibonacci sequence implementation in Go. I'd like it to be put into a file called main.go. The implementation itself should be a single function of the form `func fibonacci(n uint) []uint` which generates a slice of the first n fibonacci numbers. Inside main.go, I'd like the `main()` function to invoke `fibonacci` with the value 7 and print the results. I'd then like you to invoke main.go with `go run main.go`.
```

followed by:

```
Can you refactor main.go so that it computes the first 15 Fibonacci numbers rather than the first 7? Also please have the resulting slice reversed before printing it out. I want you to use the `slices.Reverse` function to accomplish the reversing (note that this function doesn't return a value - it sorts the slice in place, so just invoke it on the slice before printing). Pay close attention to your imports to make sure they're formatted correctly. Then, once you've updated main.go, please run it with `go run`.
```
