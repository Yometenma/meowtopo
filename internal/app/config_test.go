package app

import (
	"testing"
	"time"
)

func TestApplyStoredSettings(t *testing.T) {
	base := Config{
		CIDRs:            []string{"192.168.1.0/24"},
		ScanInterval:     5 * time.Minute,
		PingTimeout:      800 * time.Millisecond,
		TCPTimeout:       350 * time.Millisecond,
		Concurrency:      32,
		OfflineThreshold: 3,
		EnablePortScan:   true,
	}
	got, err := applyStoredSettings(base, map[string]string{
		"scan_cidrs":        "10.0.1.0/24,172.16.5.0/24",
		"scan_interval":     "15m",
		"scan_concurrency":  "64",
		"ping_timeout":      "1s",
		"tcp_timeout":       "500ms",
		"offline_threshold": "5",
		"enable_port_scan":  "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.CIDRs) != 2 || got.CIDRs[0] != "10.0.1.0/24" || got.CIDRs[1] != "172.16.5.0/24" {
		t.Fatalf("CIDRs not applied: %#v", got.CIDRs)
	}
	if got.ScanInterval != 15*time.Minute || got.Concurrency != 64 || got.PingTimeout != time.Second || got.TCPTimeout != 500*time.Millisecond || got.OfflineThreshold != 5 || got.EnablePortScan {
		t.Fatalf("settings not applied: %+v", got)
	}
}

func TestApplyStoredSettingsRejectsUnsafeValues(t *testing.T) {
	base := Config{Concurrency: 32, OfflineThreshold: 3, ScanInterval: 5 * time.Minute, PingTimeout: time.Second, TCPTimeout: time.Second}
	for name, settings := range map[string]map[string]string{
		"public CIDR":         {"scan_cidrs": "8.8.8.0/24"},
		"high concurrency":    {"scan_concurrency": "129"},
		"low threshold":       {"offline_threshold": "1"},
		"invalid interval":    {"scan_interval": "soon"},
		"invalid port toggle": {"enable_port_scan": "sometimes"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := applyStoredSettings(base, settings); err == nil {
				t.Fatal("invalid setting accepted")
			}
		})
	}
}
