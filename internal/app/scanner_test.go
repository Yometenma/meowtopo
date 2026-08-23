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
