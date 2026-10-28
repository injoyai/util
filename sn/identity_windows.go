//go:build windows

package sn

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readHardwareIdentity() (identity, error) {
	root := os.Getenv("SystemRoot")
	if !filepath.IsAbs(root) {
		return identity{}, errors.New("sn: SystemRoot is unavailable or not absolute")
	}
	command := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	data, err := hardwareCommand(command, "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference = 'Stop'; (Get-CimInstance -ClassName Win32_ComputerSystemProduct).UUID")
	if err != nil {
		return identity{}, fmt.Errorf("sn: read SMBIOS UUID through CIM: %w", err)
	}
	return identity{"smbios-uuid", strings.TrimSpace(string(data))}, nil
}
