package sn

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// 使用 fs.FS 让所有平台都能用固定样例验证 Linux 的硬件读取规则。
func readLinuxIdentity(files fs.FS) (identity, error) {
	for _, candidate := range []struct{ source, filename string }{
		{"smbios-uuid", "sys/class/dmi/id/product_uuid"},
		{"board-serial", "sys/firmware/devicetree/base/serial-number"},
	} {
		value, err := readIdentityFile(files, candidate.filename)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return identity{}, fmt.Errorf("sn: read %s: %w", candidate.source, err)
		}
		return identity{candidate.source, value}, nil
	}
	value, err := readCPUSerial(files)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return identity{}, fmt.Errorf("sn: read CPU serial: %w", err)
	}
	if value != "" {
		return identity{"cpu-serial", value}, nil
	}
	paths, err := fs.Glob(files, "sys/class/mmc_host/mmc*/mmc*:*/type")
	if err != nil {
		return identity{}, fmt.Errorf("sn: find MMC devices: %w", err)
	}
	var devices []string
	for _, filename := range paths {
		kind, err := readIdentityFile(files, filename)
		if err != nil {
			return identity{}, fmt.Errorf("sn: read MMC type: %w", err)
		}
		if kind == "MMC" {
			devices = append(devices, path.Dir(filename))
		}
	}
	if len(devices) != 1 {
		return identity{}, fmt.Errorf("sn: no unambiguous hardware identity; found %d MMC devices", len(devices))
	}
	value, err = readIdentityFile(files, path.Join(devices[0], "cid"))
	if err != nil {
		return identity{}, fmt.Errorf("sn: read MMC CID: %w", err)
	}
	return identity{"mmc-cid", value}, nil
}

func readIdentityFile(files fs.FS, filename string) (string, error) {
	data, err := fs.ReadFile(files, filename)
	if err != nil {
		return "", err
	}
	value := strings.Trim(string(data), "\x00 \t\r\n")
	if value == "" {
		return "", errors.New("identity file is empty")
	}
	return value, nil
}

func readCPUSerial(files fs.FS) (string, error) {
	file, err := files.Open("proc/cpuinfo")
	if err != nil {
		return "", err
	}
	defer file.Close()
	var serial string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "serial") {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || (serial != "" && serial != value) {
			return "", errors.New("CPU serial is empty or ambiguous")
		}
		serial = value
	}
	return serial, scanner.Err()
}
