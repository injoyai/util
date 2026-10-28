package sn

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlatformUUID(t *testing.T) {
	for _, data := range []string{
		`    "IOPlatformUUID" = "00112233-4455-6677-8899-aabbccddeeff"`,
		"+-o IOPlatformExpertDevice\n  | {\n  |   \"IOPlatformUUID\" = \"00112233445566778899AABBCCDDEEFF\"\n  | }",
	} {
		id, err := parsePlatformUUID(data)
		if err != nil {
			t.Fatal(err)
		}
		got, err := derive(id)
		if err != nil || got != "3D48D730A058557F" {
			t.Fatalf("platform UUID parse changed SN: %q, %v", got, err)
		}
	}
	for _, data := range []string{"", `"OtherUUID" = "123"`, `"IOPlatformUUID" = invalid`} {
		if _, err := parsePlatformUUID(data); err == nil {
			t.Fatal("missing or malformed platform UUID accepted")
		}
	}
}

func TestHardwareCommand(t *testing.T) {
	data, err := hardwareCommand(os.Args[0], "-test.run=^TestHardwareCommandHelper$", "--", "sn-command-success")
	if err != nil || string(data) != "fixture" {
		t.Fatalf("command output: %q, %v", data, err)
	}
	if _, err := hardwareCommand(os.Args[0], "-test.run=^TestHardwareCommandHelper$", "--", "sn-command-failure"); err == nil {
		t.Fatal("command failure ignored")
	}
	if _, err := hardwareCommand(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing command accepted")
	}
}

func TestHardwareCommandHelper(t *testing.T) {
	switch os.Args[len(os.Args)-1] {
	case "sn-command-success":
		fmt.Print("fixture")
		os.Exit(0)
	case "sn-command-failure":
		os.Exit(7)
	}
}
