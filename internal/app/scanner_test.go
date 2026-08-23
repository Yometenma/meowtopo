package app

import "testing"

func TestProbePortsCanBeDisabled(t *testing.T) {
	if ports := probePorts(false); len(ports) != 0 {
		t.Fatalf("disabled port scan returned ports: %v", ports)
	}
	if ports := probePorts(true); len(ports) == 0 {
		t.Fatal("enabled port scan returned no ports")
	}
}

func TestIdentifyType(t *testing.T) {
	tests := []struct {
		name, host, wantType, wantSource string
		ports                            []int
	}{
		{"hostname NAS", "DiskStation-Synology.local.", "nas", "hostname", nil},
		{"hostname router", "home-openwrt", "router", "hostname", nil},
		{"Home Assistant port", "", "iot", "ports", []int{8123}},
		{"camera port", "", "camera", "ports", []int{554}},
		{"DNS web gateway", "", "router", "ports", []int{53, 443}},
		{"unknown", "living-room", "unknown", "", []int{80}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotType, gotSource, confidence := identifyType(test.host, test.ports)
			if gotType != test.wantType || gotSource != test.wantSource {
				t.Fatalf("got %s/%s, want %s/%s", gotType, gotSource, test.wantType, test.wantSource)
			}
			if gotType != "unknown" && confidence <= 0 {
				t.Fatal("identified device has no confidence")
			}
		})
	}
}
