package app

import "testing"

func TestParseMDNSPTR(t *testing.T) {
	message := []byte{0, 0, 0x84, 0, 0, 0, 0, 1, 0, 0, 0, 0}
	message = append(message, 2, '2', '0', 1, '1', 3, '1', '6', '8', 3, '1', '9', '2', 7, 'i', 'n', '-', 'a', 'd', 'd', 'r', 4, 'a', 'r', 'p', 'a', 0)
	message = append(message, 0, 12, 0, 1, 0, 0, 0, 120, 0, 22, 14, 'l', 'i', 'v', 'i', 'n', 'g', '-', 'r', 'o', 'o', 'm', '-', 't', 'v', 5, 'l', 'o', 'c', 'a', 'l', 0)
	if got := parseMDNSPTR(message); got != "living-room-tv.local" {
		t.Fatalf("name=%q", got)
	}
}
