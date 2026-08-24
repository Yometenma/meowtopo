package app

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"time"
)

// lookupMDNSName asks a reachable local device for its Bonjour/mDNS reverse name.
// The unicast-response bit lets us use an ephemeral local port without running a daemon.
func lookupMDNSName(ip string, sourceIP net.IP, timeout time.Duration) string {
	parsed := net.ParseIP(ip).To4()
	if parsed == nil {
		return ""
	}
	parts := []string{strconv.Itoa(int(parsed[3])), strconv.Itoa(int(parsed[2])), strconv.Itoa(int(parsed[1])), strconv.Itoa(int(parsed[0])), "in-addr", "arpa"}
	name := strings.Join(parts, ".")
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[4:6], 1)
	for _, label := range strings.Split(name, ".") {
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	packet = append(packet, 0, 0, 12, 0x80, 1) // PTR, IN, request unicast reply
	local := &net.UDPAddr{IP: sourceIP, Port: 0}
	conn, err := net.DialUDP("udp4", local, &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353})
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.Write(packet); err != nil {
		return ""
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	return parseMDNSPTR(buf[:n])
}

func parseMDNSPTR(message []byte) string {
	records := parseMDNSPTRRecords(message)
	if len(records) > 0 {
		return records[0].Target
	}
	return ""
}

type mdnsPTRRecord struct {
	Name   string
	Target string
}

func parseMDNSPTRRecords(message []byte) []mdnsPTRRecord {
	if len(message) < 12 {
		return nil
	}
	questions := int(binary.BigEndian.Uint16(message[4:6]))
	records := int(binary.BigEndian.Uint16(message[6:8])) + int(binary.BigEndian.Uint16(message[8:10])) + int(binary.BigEndian.Uint16(message[10:12]))
	offset := 12
	for range questions {
		_, next, ok := dnsName(message, offset, 0)
		if !ok || next+4 > len(message) {
			return nil
		}
		offset = next + 4
	}
	var result []mdnsPTRRecord
	for range records {
		owner, next, ok := dnsName(message, offset, 0)
		if !ok || next+10 > len(message) {
			return result
		}
		typ := binary.BigEndian.Uint16(message[next : next+2])
		length := int(binary.BigEndian.Uint16(message[next+8 : next+10]))
		data := next + 10
		if data+length > len(message) {
			return result
		}
		if typ == 12 {
			name, _, valid := dnsName(message, data, 0)
			if valid {
				result = append(result, mdnsPTRRecord{Name: strings.TrimSuffix(owner, "."), Target: strings.TrimSuffix(name, ".")})
			}
		}
		offset = data + length
	}
	return result
}

func dnsName(message []byte, offset, depth int) (string, int, bool) {
	if depth > 12 || offset >= len(message) {
		return "", offset, false
	}
	labels := []string{}
	next := offset
	jumped := false
	for {
		if offset >= len(message) {
			return "", next, false
		}
		length := int(message[offset])
		if length&0xc0 == 0xc0 {
			if offset+1 >= len(message) {
				return "", next, false
			}
			pointer := (length&0x3f)<<8 | int(message[offset+1])
			name, _, ok := dnsName(message, pointer, depth+1)
			if !ok {
				return "", next, false
			}
			labels = append(labels, strings.TrimSuffix(name, "."))
			if !jumped {
				next = offset + 2
			}
			return strings.Join(labels, ".") + ".", next, true
		}
		offset++
		if length == 0 {
			if !jumped {
				next = offset
			}
			return strings.Join(labels, ".") + ".", next, true
		}
		if length > 63 || offset+length > len(message) {
			return "", next, false
		}
		labels = append(labels, string(message[offset:offset+length]))
		offset += length
		if !jumped {
			next = offset
		}
	}
}
