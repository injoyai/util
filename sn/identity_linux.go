//go:build linux

package sn

import "os"

func readHardwareIdentity() (identity, error) {
	return readLinuxIdentity(os.DirFS("/"))
}
