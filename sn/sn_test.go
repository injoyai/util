package sn

import (
	"strings"
	"testing"
)

func TestDeriveGolden(t *testing.T) {
	for _, tc := range []struct {
		id   identity
		want string
	}{
		{identity{"smbios-uuid", "00112233-4455-6677-8899-aabbccddeeff"}, "2561E21F9D632041"},
		{identity{"platform-uuid", "00112233445566778899AABBCCDDEEFF"}, "3D48D730A058557F"},
		{identity{"board-serial", "Board-ab-123456"}, "EAA67F69BA7685EC"},
		{identity{"cpu-serial", "00000000765fc593"}, "5006133D6EB61FF7"},
		{identity{"mmc-cid", "150100514536334d42003a1b2c3d4e5f"}, "B9305A692D096595"},
	} {
		got, err := derive(tc.id)
		if err != nil || got != tc.want {
			t.Fatalf("derive(%s) = %q, %v; want %q", tc.id.source, got, err, tc.want)
		}
	}
}

func TestUUIDNormalization(t *testing.T) {
	want, err := derive(identity{"smbios-uuid", "00112233-4455-6677-8899-aabbccddeeff"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"00112233445566778899AABBCCDDEEFF",
		" \t00112233-4455-6677-8899-AABBCCDDEEFF\n",
	} {
		got, err := derive(identity{"smbios-uuid", value})
		if err != nil || got != want {
			t.Fatalf("UUID presentation changed SN: %q, %v", got, err)
		}
	}
}

func TestDerivePreservesSerialAndSource(t *testing.T) {
	seen := make(map[string]bool)
	for _, id := range []identity{
		{"board-serial", "AB-123456"},
		{"board-serial", "A-B123456"},
		{"board-serial", "ab-123456"},
		{"board-serial", "AB123456"},
		{"cpu-serial", "AB123456"},
	} {
		got, err := derive(id)
		if err != nil {
			t.Fatal(err)
		}
		if seen[got] {
			t.Fatal("distinct serials or sources produced duplicate SN")
		}
		seen[got] = true
	}
}

func TestDeriveInvalidIdentity(t *testing.T) {
	for _, id := range []identity{
		{"", "AB-123456"},
		{"board-serial", ""},
		{"board-serial", " \t\r\n"},
		{"board-serial", "AB\x00CD"},
		{"board-serial", "To be filled by O.E.M."},
		{"cpu-serial", "UNKNOWN"},
		{"cpu-serial", "0000000000000000"},
		{"smbios-uuid", "00000000-0000-0000-0000-000000000000"},
		{"smbios-uuid", "FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF"},
		{"smbios-uuid", "00112233-44556677-8899-aabb-ccddeeff"},
		{"smbios-uuid", "00112233445566778899AABBCCDDEEFG"},
		{"mmc-cid", strings.Repeat("F", 32)},
		{"mmc-cid", "1234"},
	} {
		if got, err := derive(id); err == nil || got != "" {
			t.Fatalf("invalid %s accepted: %q, %v", id.source, got, err)
		}
	}
}
