//go:build !linux && !darwin && !windows

package sn

import (
	"fmt"
	"runtime"
)

func readHardwareIdentity() (identity, error) {
	return identity{}, fmt.Errorf("sn: hardware identity is unsupported on %s", runtime.GOOS)
}
