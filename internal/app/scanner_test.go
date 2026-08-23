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

func TestARPResponseMarksLocalDeviceAlive(t *testing.T) {
	result := applyARPResult(ProbeResult{}, "aa:bb:cc:dd:ee:ff")
	if !result.Alive || result.Method != "arp" {
		t.Fatalf("ARP neighbor was not accepted: %+v", result)
	}
	result = applyARPResult(ProbeResult{Alive: true, Method: "icmp", Latency: 2}, "aa:bb:cc:dd:ee:ff")
	if result.Method != "icmp" || result.Latency != 2 {
		t.Fatalf("ARP overwrote stronger probe: %+v", result)
	}
}

func TestParseARPLineRequiresCompleteNeighbor(t *testing.T) {
	if got := parseARPLine("192.168.1.20 0x1 0x2 aa:bb:cc:dd:ee:ff * eth0", "192.168.1.20"); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("valid ARP entry=%q", got)
	}
	for _, line := range []string{
		"192.168.1.20 0x1 0x0 00:00:00:00:00:00 * eth0",
		"192.168.1.20 0x1 0x2 ff:ff:ff:ff:ff:ff * eth0",
	} {
		if got := parseARPLine(line, "192.168.1.20"); got != "" {
			t.Fatalf("incomplete ARP entry accepted: %q", got)
		}
	}
}

func TestParseWindowsARP(t *testing.T) {
	neighbors := parseWindowsARP("接口: 192.168.1.10 --- 0x8\r\n  192.168.1.1          aa-bb-cc-dd-ee-01     动态\r\n  192.168.1.20         00-00-00-00-00-00     无效\r\n")
	if neighbors["192.168.1.1"] != "aa:bb:cc:dd:ee:01" || len(neighbors) != 1 {
		t.Fatalf("neighbors=%v", neighbors)
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
