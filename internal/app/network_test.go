package app

import "testing"

func TestValidateCIDR(t *testing.T) {
	for _, v := range []string{"192.168.1.0/24", "10.2.0.0/23", "172.16.3.0/24"} {
		if _, _, e := validateCIDR(v); e != nil {
			t.Fatalf("%s: %v", v, e)
		}
	}
	for _, v := range []string{"8.8.8.0/24", "192.168.0.0/16", "10.0.0.0/8", "bad"} {
		if _, _, e := validateCIDR(v); e == nil {
			t.Fatalf("should reject %s", v)
		}
	}
}
func TestNormalizeMAC(t *testing.T) {
	got, e := normalizeMAC("AA-BB-CC-DD-EE-FF")
	if e != nil || got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("got %q %v", got, e)
	}
	if _, e = normalizeMAC("oops"); e == nil {
		t.Fatal("invalid MAC accepted")
	}
}

func TestInterfaceIPv4(t *testing.T) {
	if ip, err := interfaceIPv4(""); err != nil || ip != nil {
		t.Fatalf("automatic interface should not bind a source IP: %v %v", ip, err)
	}
	if _, err := interfaceIPv4("meowtopo-interface-that-does-not-exist"); err == nil {
		t.Fatal("missing interface accepted")
	}
}
