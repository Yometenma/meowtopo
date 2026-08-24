package app

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type networkEvidence map[string][]identificationEvidence

func discoverServiceEvidence(ctx context.Context, sourceIP net.IP, timeout time.Duration) networkEvidence {
	result := networkEvidence{}
	var mu sync.Mutex
	merge := func(found networkEvidence) {
		mu.Lock()
		defer mu.Unlock()
		for ip, evidence := range found {
			result[ip] = append(result[ip], evidence...)
		}
	}
	var wait sync.WaitGroup
	for _, discover := range []func(context.Context, net.IP, time.Duration) networkEvidence{discoverMDNSServices, discoverSSDPServices} {
		wait.Add(1)
		go func(discover func(context.Context, net.IP, time.Duration) networkEvidence) {
			defer wait.Done()
			merge(discover(ctx, sourceIP, timeout))
		}(discover)
	}
	wait.Wait()
	return result
}

var mdnsServiceTypes = []string{
	"_airplay._tcp.local", "_googlecast._tcp.local", "_ipp._tcp.local", "_printer._tcp.local",
	"_hap._tcp.local", "_homekit._tcp.local", "_raop._tcp.local",
}

func discoverMDNSServices(ctx context.Context, sourceIP net.IP, timeout time.Duration) networkEvidence {
	result := networkEvidence{}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: sourceIP, Port: 0})
	if err != nil {
		return result
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	for _, service := range mdnsServiceTypes {
		packet := dnsPTRQuery(service)
		_, _ = conn.WriteToUDP(packet, &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353})
	}
	buffer := make([]byte, 65535)
	for {
		if ctx.Err() != nil {
			return result
		}
		n, remote, readErr := conn.ReadFromUDP(buffer)
		if readErr != nil {
			return result
		}
		ip := remote.IP.To4()
		if ip == nil || !isPrivateIPv4(ip) {
			continue
		}
		for _, record := range parseMDNSPTRRecords(buffer[:n]) {
			if evidence, ok := mdnsServiceEvidence(record.Name, record.Target); ok {
				result[ip.String()] = appendUniqueEvidence(result[ip.String()], evidence)
			}
		}
	}
}

func dnsPTRQuery(name string) []byte {
	packet := make([]byte, 12)
	packet[5] = 1
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	return append(packet, 0, 0, 12, 0x80, 1)
}

func mdnsServiceEvidence(name, target string) (identificationEvidence, bool) {
	service := strings.ToLower(name + " " + target)
	switch {
	case strings.Contains(service, "_ipp._tcp") || strings.Contains(service, "_printer._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "AirPrint/IPP 打印服务", DeviceType: "printer", Weight: .94}, true
	case strings.Contains(service, "_airplay._tcp") || strings.Contains(service, "_raop._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "AirPlay 媒体服务", DeviceType: "tv", Weight: .88}, true
	case strings.Contains(service, "_googlecast._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "Google Cast 服务", DeviceType: "tv", Weight: .93}, true
	case strings.Contains(service, "_hap._tcp") || strings.Contains(service, "_homekit._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "HomeKit 服务", DeviceType: "iot", Weight: .88}, true
	default:
		return identificationEvidence{}, false
	}
}

func discoverSSDPServices(ctx context.Context, sourceIP net.IP, timeout time.Duration) networkEvidence {
	result := networkEvidence{}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: sourceIP, Port: 0})
	if err != nil {
		return result
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	request := strings.Join([]string{
		"M-SEARCH * HTTP/1.1", "HOST: 239.255.255.250:1900", `MAN: "ssdp:discover"`, "MX: 1", "ST: ssdp:all", "", "",
	}, "\r\n")
	_, _ = conn.WriteToUDP([]byte(request), &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
	buffer := make([]byte, 65535)
	for {
		if ctx.Err() != nil {
			return result
		}
		n, remote, readErr := conn.ReadFromUDP(buffer)
		if readErr != nil {
			return result
		}
		headers, parseErr := parseSSDPResponse(buffer[:n])
		if parseErr != nil {
			continue
		}
		ip := ssdpResponseIP(headers, remote.IP)
		if ip == nil || !isPrivateIPv4(ip) {
			continue
		}
		if evidence, ok := ssdpServiceEvidence(headers); ok {
			result[ip.String()] = appendUniqueEvidence(result[ip.String()], evidence)
		}
	}
}

func parseSSDPResponse(message []byte) (http.Header, error) {
	response, err := http.ReadResponse(bufio.NewReader(strings.NewReader(string(message))), nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected SSDP response: %s", response.Status)
	}
	return response.Header, nil
}

func ssdpResponseIP(headers http.Header, fallback net.IP) net.IP {
	location, err := url.Parse(headers.Get("Location"))
	if err == nil && location.Hostname() != "" {
		if parsed := net.ParseIP(location.Hostname()).To4(); parsed != nil {
			return parsed
		}
	}
	return fallback.To4()
}

func ssdpServiceEvidence(headers http.Header) (identificationEvidence, bool) {
	description := strings.ToLower(strings.Join([]string{headers.Get("ST"), headers.Get("USN"), headers.Get("Server")}, " "))
	switch {
	case strings.Contains(description, "internetgatewaydevice") || strings.Contains(description, "wanipconnection") || strings.Contains(description, "wanpppconnection"):
		return identificationEvidence{Kind: "ssdp", Value: "UPnP Internet 网关服务", DeviceType: "router", Weight: .95}, true
	case strings.Contains(description, "mediarenderer") || strings.Contains(description, "roku") || strings.Contains(description, "dial"):
		return identificationEvidence{Kind: "ssdp", Value: "UPnP 媒体播放服务", DeviceType: "tv", Weight: .9}, true
	case strings.Contains(description, "printer"):
		return identificationEvidence{Kind: "ssdp", Value: "UPnP 打印服务", DeviceType: "printer", Weight: .92}, true
	default:
		return identificationEvidence{}, false
	}
}

func appendUniqueEvidence(items []identificationEvidence, candidate identificationEvidence) []identificationEvidence {
	for _, item := range items {
		if item.Kind == candidate.Kind && item.Value == candidate.Value && item.DeviceType == candidate.DeviceType {
			return items
		}
	}
	return append(items, candidate)
}

func isPrivateIPv4(ip net.IP) bool {
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
