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

type discoveredServices struct {
	evidence  networkEvidence
	hostnames map[string]string
}

func discoverServiceEvidence(ctx context.Context, sourceIP net.IP, timeout time.Duration) (networkEvidence, map[string]string) {
	evidence := networkEvidence{}
	hostnames := map[string]string{}
	var mu sync.Mutex
	merge := func(found discoveredServices) {
		mu.Lock()
		defer mu.Unlock()
		for ip, items := range found.evidence {
			evidence[ip] = append(evidence[ip], items...)
		}
		for ip, name := range found.hostnames {
			if name != "" && hostnames[ip] == "" {
				hostnames[ip] = name
			}
		}
	}
	var wait sync.WaitGroup
	for _, discover := range []func(context.Context, net.IP, time.Duration) discoveredServices{discoverMDNSServices, discoverSSDPServices} {
		wait.Add(1)
		go func(discover func(context.Context, net.IP, time.Duration) discoveredServices) {
			defer wait.Done()
			merge(discover(ctx, sourceIP, timeout))
		}(discover)
	}
	wait.Wait()
	return evidence, hostnames
}

// mdnsInstanceName 从 mDNS 域名里提取人类可读的实例名或主机名。
// 例如 "客厅的电视._googlecast._tcp.local" → "客厅的电视"，
// "my-nas.local" → "my-nas"；纯服务类型名（以 "_" 开头）视为无效。
func mdnsInstanceName(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	lower := strings.ToLower(name)
	for _, proto := range []string{"._tcp", "._udp"} {
		if idx := strings.Index(lower, proto); idx > 0 {
			instance := name[:idx]
			if i := strings.Index(instance, "._"); i > 0 {
				instance = instance[:i]
			}
			instance = strings.TrimSpace(instance)
			if instance != "" && !strings.HasPrefix(instance, "_") {
				return instance
			}
			return ""
		}
	}
	name = strings.TrimSuffix(name, ".local")
	if name == "" || strings.HasPrefix(name, "_") {
		return ""
	}
	return name
}

var mdnsServiceTypes = []string{
	"_airplay._tcp.local", "_googlecast._tcp.local", "_ipp._tcp.local", "_printer._tcp.local",
	"_hap._tcp.local", "_homekit._tcp.local", "_raop._tcp.local",
	"_smb._tcp.local", "_nfs._tcp.local", "_afpovertcp._tcp.local",
	"_ssh._tcp.local", "_sftp-ssh._tcp.local",
	"_sonos._tcp.local", "_spotify-connect._tcp.local",
	"_amzn-wplay._tcp.local",
	"_dlna._tcp.local", "_daap._tcp.local", "_touch-able._tcp.local",
	"_appletv-v2._tcp.local", "_mediaremotetv._tcp.local",
	"_rfb._tcp.local", "_ard._tcp.local",
	"_adb._tcp.local",
	"_device-info._tcp.local",
	"_workstation._tcp.local",
}

func discoverMDNSServices(ctx context.Context, sourceIP net.IP, timeout time.Duration) discoveredServices {
	result := discoveredServices{evidence: networkEvidence{}, hostnames: map[string]string{}}
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
			if name := mdnsInstanceName(record.Target); name != "" && result.hostnames[ip.String()] == "" {
				result.hostnames[ip.String()] = name
			}
			if evidence, ok := mdnsServiceEvidence(record.Name, record.Target); ok {
				result.evidence[ip.String()] = appendUniqueEvidence(result.evidence[ip.String()], evidence)
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
	case strings.Contains(service, "_smb._tcp") || strings.Contains(service, "_nfs._tcp") || strings.Contains(service, "_afpovertcp._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "文件共享服务（SMB/NFS/AFP）", DeviceType: "nas", Weight: .93}, true
	case strings.Contains(service, "_sonos._tcp") || strings.Contains(service, "_spotify-connect._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "音频串流服务", DeviceType: "tv", Weight: .85}, true
	case strings.Contains(service, "_amzn-wplay._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "Amazon 设备服务", DeviceType: "iot", Weight: .9}, true
	case strings.Contains(service, "_dlna._tcp") || strings.Contains(service, "_daap._tcp") || strings.Contains(service, "_touch-able._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "DLNA 媒体服务", DeviceType: "tv", Weight: .82}, true
	case strings.Contains(service, "_appletv-v2._tcp") || strings.Contains(service, "_mediaremotetv._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "Apple TV 服务", DeviceType: "tv", Weight: .94}, true
	case strings.Contains(service, "_rfb._tcp") || strings.Contains(service, "_ard._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "远程桌面服务", DeviceType: "macos", Weight: .72}, true
	case strings.Contains(service, "_adb._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "Android 调试服务", DeviceType: "phone", Weight: .9}, true
	case strings.Contains(service, "_ssh._tcp") || strings.Contains(service, "_sftp-ssh._tcp"):
		return identificationEvidence{Kind: "mdns", Value: "SSH 服务", DeviceType: "linux", Weight: .5}, true
	default:
		return identificationEvidence{}, false
	}
}

func discoverSSDPServices(ctx context.Context, sourceIP net.IP, timeout time.Duration) discoveredServices {
	result := discoveredServices{evidence: networkEvidence{}, hostnames: map[string]string{}}
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
			result.evidence[ip.String()] = appendUniqueEvidence(result.evidence[ip.String()], evidence)
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
	case strings.Contains(description, "mediaserver"):
		return identificationEvidence{Kind: "ssdp", Value: "UPnP 媒体服务器服务", DeviceType: "nas", Weight: .8}, true
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
