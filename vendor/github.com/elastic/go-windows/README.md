# go-windows

[![ci](https://github.com/elastic/go-windows/actions/workflows/ci.yml/badge.svg)](https://github.com/elastic/go-windows/actions/workflows/ci.yml)
[![Go Documentation](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)][godocs]

[godocs]: https://pkg.go.dev/github.com/elastic/go-windows?GOOS=windows

go-windows is a library for Go (golang) that provides wrappers to various
Windows APIs that are not covered by the stdlib or by
[golang.org/x/sys/windows](https://godoc.org/golang.org/x/sys/windows).

Goals / Features

- Does not use cgo.
- Provide abstractions to make using the APIs easier.
