package app

import (
	"net"
	"testing"
)

func TestMDNSServiceEvidence(t *testing.T) {
	tests := []struct {
		service, want string
	}{
		{"_ipp._tcp.local", "printer"},
		{"_googlecast._tcp.local", "tv"},
		{"_hap._tcp.local", "iot"},
		{"_smb._tcp.local", "nas"},
		{"_nfs._tcp.local", "nas"},
		{"_afpovertcp._tcp.local", "nas"},
		{"_amzn-wplay._tcp.local", "iot"},
		{"_appletv-v2._tcp.local", "tv"},
		{"_adb._tcp.local", "phone"},
		{"_ssh._tcp.local", "linux"},
	}
	for _, test := range tests {
		evidence, ok := mdnsServiceEvidence(test.service, "living-room.local")
		if !ok || evidence.DeviceType != test.want || evidence.Kind != "mdns" {
			t.Errorf("service %q produced %+v, ok=%v", test.service, evidence, ok)
		}
	}
}

func TestParseSSDPResponseAndEvidence(t *testing.T) {
	message := "HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\nUSN: uuid:fictional::upnp:rootdevice\r\nLOCATION: http://192.168.7.1:1900/device.xml\r\nSERVER: FictionOS UPnP/1.0\r\n\r\n"
	headers, err := parseSSDPResponse([]byte(message))
	if err != nil {
		t.Fatal(err)
	}
	if ip := ssdpResponseIP(headers, net.ParseIP("192.168.7.2")); ip.String() != "192.168.7.1" {
		t.Fatalf("location IP=%v", ip)
	}
	evidence, ok := ssdpServiceEvidence(headers)
	if !ok || evidence.DeviceType != "router" || evidence.Kind != "ssdp" {
		t.Fatalf("unexpected evidence: %+v, ok=%v", evidence, ok)
	}
}

func TestSSDPRejectsUnknownAndPublicTargets(t *testing.T) {
	headers, err := parseSSDPResponse([]byte("HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ssdpServiceEvidence(headers); ok {
		t.Fatal("generic root device was assigned a type")
	}
	if isPrivateIPv4(net.ParseIP("203.0.113.10")) || !isPrivateIPv4(net.ParseIP("192.168.7.10")) {
		t.Fatal("private network boundary is incorrect")
	}
}

func TestServiceEvidenceJoinsIdentification(t *testing.T) {
	extra := identificationEvidence{Kind: "ssdp", Value: "UPnP 媒体播放服务", DeviceType: "tv", Weight: .9}
	result := identifyDevice("living-room", "00:11:22:33:44:55", []int{8008}, extra)
	if result.DeviceType != "tv" || result.Source != "ports+ssdp" || result.Confidence <= .9 {
		t.Fatalf("service evidence was not combined: %+v", result)
	}
}

func TestMDNSInstanceName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"客厅的电视._googlecast._tcp.local", "客厅的电视"},
		{"my-nas.local", "my-nas"},
		{"Living Room Apple TV._airplay._tcp.local", "Living Room Apple TV"},
		{"_googlecast._tcp.local", ""},
	}
	for _, test := range tests {
		if got := mdnsInstanceName(test.input); got != test.want {
			t.Errorf("mdnsInstanceName(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestExpandedHostnameRules(t *testing.T) {
	tests := []struct {
		host, want string
	}{
		{"redmi-note-10", "phone"},
		{"tcl-65-inch", "tv"},
		{"hikvision-nvr", "camera"},
		{"terramaster-f4", "nas"},
		{"miwifi-r4a", "router"},
		{"echo-dot-3", "iot"},
	}
	for _, test := range tests {
		if got := identifyDevice(test.host, "", nil).DeviceType; got != test.want {
			t.Errorf("identifyDevice(%q) = %q, want %q", test.host, got, test.want)
		}
	}
}
