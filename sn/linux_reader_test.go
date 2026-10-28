package sn

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLinuxHardwareSources(t *testing.T) {
	const uuid = "00112233-4455-6677-8899-aabbccddeeff"
	const cid = "150100514536334d42003a1b2c3d4e5f"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"SMBIOS takes priority", map[string]string{
			"sys/class/dmi/id/product_uuid":              uuid + "\n",
			"sys/firmware/devicetree/base/serial-number": "Board-ab-123456\x00",
		}, "2561E21F9D632041"},
		{"generic device tree", map[string]string{
			"sys/firmware/devicetree/base/serial-number": "Board-ab-123456\x00\n",
		}, "EAA67F69BA7685EC"},
		{"CPU serial", map[string]string{
			"proc/cpuinfo": "processor : 0\nSerial\t: 00000000765fc593\n",
		}, "5006133D6EB61FF7"},
		{"MMC on another host excludes SD", map[string]string{
			"proc/cpuinfo":                           "processor : 0\n",
			"sys/class/mmc_host/mmc0/mmc0:0001/type": "SD\n",
			"sys/class/mmc_host/mmc0/mmc0:0001/cid":  strings.Repeat("1", 32),
			"sys/class/mmc_host/mmc2/mmc2:0001/type": "MMC\n",
			"sys/class/mmc_host/mmc2/mmc2:0001/cid":  cid + "\n",
		}, "B9305A692D096595"},
		{"no hardware identity", nil, ""},
		{"invalid UUID does not fall back", map[string]string{
			"sys/class/dmi/id/product_uuid":              "00000000-0000-0000-0000-000000000000",
			"sys/firmware/devicetree/base/serial-number": "Board-ab-123456",
		}, ""},
		{"empty device tree does not fall back", map[string]string{
			"sys/firmware/devicetree/base/serial-number": "\x00\n",
			"proc/cpuinfo": "Serial: 00000000765fc593\n",
		}, ""},
		{"multiple CPU serials", map[string]string{
			"proc/cpuinfo": "Serial: ABC123\nSerial: DEF456\n",
		}, ""},
		{"empty CPU serial", map[string]string{
			"proc/cpuinfo": "Serial: \n",
		}, ""},
		{"CPU scanner failure", map[string]string{
			"proc/cpuinfo": "Serial: ABC123\n" + strings.Repeat("x", 100000),
		}, ""},
		{"SD alone is not MMC", map[string]string{
			"sys/class/mmc_host/mmc0/mmc0:0001/type": "SD",
			"sys/class/mmc_host/mmc0/mmc0:0001/cid":  cid,
		}, ""},
		{"multiple MMC devices", map[string]string{
			"sys/class/mmc_host/mmc0/mmc0:0001/type": "MMC",
			"sys/class/mmc_host/mmc1/mmc1:0001/type": "MMC",
		}, ""},
		{"MMC CID unreadable", map[string]string{
			"sys/class/mmc_host/mmc0/mmc0:0001/type": "MMC",
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := fstest.MapFS{}
			for name, data := range tc.files {
				files[name] = &fstest.MapFile{Data: []byte(data)}
			}
			id, err := readLinuxIdentity(files)
			var got string
			if err == nil {
				got, err = derive(id)
			}
			if tc.want == "" {
				if err == nil || got != "" {
					t.Fatalf("expected failure, got %q, %v", got, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

type deniedFS struct {
	fs.FS
	path string
}

func (f deniedFS) Open(name string) (fs.File, error) {
	if name == f.path {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.FS.Open(name)
}

func TestLinuxPermissionDoesNotChangeSource(t *testing.T) {
	files := fstest.MapFS{
		"sys/firmware/devicetree/base/serial-number": {Data: []byte("Board-ab-123456")},
	}
	_, err := readLinuxIdentity(deniedFS{files, "sys/class/dmi/id/product_uuid"})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("permission error lost or source changed: %v", err)
	}
}
