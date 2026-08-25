package app

import (
	"strings"
	"testing"
)

func TestReadIEEEVendorCSVAndLongestPrefix(t *testing.T) {
	csvData := "Registry,Assignment,Organization Name,Organization Address\nMA-L,001122,Example Networks,Test Street\nMA-M,0011223,Example Devices,Test Street\nMA-S,001122334,Example Small Blocks,Test Street\n"
	entries := map[string]string{}
	if err := readIEEEVendorCSV(strings.NewReader(csvData), entries); err != nil {
		t.Fatal(err)
	}
	database := &macVendorDatabase{entries: entries}
	tests := map[string]string{
		"00:11:22:aa:bb:cc": "Example Networks",
		"00:11:22:3a:bb:cc": "Example Devices",
		"00:11:22:33:4b:cc": "Example Small Blocks",
	}
	for mac, want := range tests {
		if got := database.Lookup(mac); got != want {
			t.Errorf("Lookup(%s)=%q, want %q", mac, got, want)
		}
	}
}

func TestMACVendorDatabasePersistsLocally(t *testing.T) {
	database := openMACVendorDatabase(t.TempDir())
	entries := map[string]string{"001122": "Example Networks"}
	if err := writeVendorDatabase(database.path, entries); err != nil {
		t.Fatal(err)
	}
	entries["AABBCC"] = "Updated Vendor"
	if err := writeVendorDatabase(database.path, entries); err != nil {
		t.Fatalf("second update failed: %v", err)
	}
	if err := database.load(); err != nil {
		t.Fatal(err)
	}
	if got := database.Lookup("00:11:22:aa:bb:cc"); got != "Example Networks" {
		t.Fatalf("vendor=%q", got)
	}
	if got := database.Lookup("02:11:22:aa:bb:cc"); got != "" {
		t.Fatalf("random MAC received vendor %q", got)
	}
}
