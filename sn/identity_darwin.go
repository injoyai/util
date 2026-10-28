//go:build darwin

package sn

import "fmt"

func readHardwareIdentity() (identity, error) {
	data, err := hardwareCommand("/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	if err != nil {
		return identity{}, fmt.Errorf("sn: read IOPlatformUUID: %w", err)
	}
	return parsePlatformUUID(string(data))
}
