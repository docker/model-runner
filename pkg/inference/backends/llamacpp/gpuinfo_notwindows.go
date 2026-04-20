//go:build !windows

package llamacpp

import "context"

func CanUseGPU(context.Context, string) (bool, error) { return false, nil }
