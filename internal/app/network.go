package app

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type InterfaceInfo struct {
	Name        string `json:"name"`
	IP          string `json:"ip"`
	Mask        string `json:"mask"`
	CIDR        string `json:"cidr"`
	Virtual     bool   `json:"virtual"`
	Recommended bool   `json:"recommended"`
}
type Detection struct {
	Interfaces        []InterfaceInfo `json:"interfaces"`
	Gateways          []string        `json:"gateways"`
	DockerLikely      bool            `json:"docker_likely"`
	RawProbeAvailable bool            `json:"raw_probe_available"`
	DataWritable      bool            `json:"data_writable"`
	Warnings          []string        `json:"warnings"`
}

func detectNetwork(dataDir string) Detection {
	d := Detection{RawProbeAvailable: runtime.GOOS == "linux"}
	ifs, _ := net.Interfaces()
	for _, in := range ifs {
		if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := in.Addrs()
		v := isVirtual(in.Name)
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ones, _ := ipn.Mask.Size()
			cidr := ipn.String()
			d.Interfaces = append(d.Interfaces, InterfaceInfo{Name: in.Name, IP: ipn.IP.String(), Mask: net.IP(ipn.Mask).String(), CIDR: cidr, Virtual: v, Recommended: ipn.IP.IsPrivate() && !v && ones >= 20})
		}
	}
	d.Gateways = linuxGateways()
	for _, i := range d.Interfaces {
		if i.Virtual && strings.Contains(strings.ToLower(i.Name), "docker") {
			d.DockerLikely = true
		}
	}
	f, e := os.CreateTemp(dataDir, "write-test-")
	if e == nil {
		d.DataWritable = true
		f.Close()
		os.Remove(f.Name())
	}
	if d.DockerLikely {
		d.Warnings = append(d.Warnings, "检测到 Docker 虚拟网卡；容器部署建议使用 host 网络模式。")
	}
	if len(d.Gateways) == 0 {
		d.Warnings = append(d.Warnings, "未检测到默认网关，可在向导中手工填写。")
	}
	return d
}
func isVirtual(n string) bool {
	n = strings.ToLower(n)
	for _, p := range []string{"docker", "br-", "veth", "virbr", "tun", "tap", "tailscale", "wg", "vpn", "loopback"} {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}
func interfaceIPv4(name string) (net.IP, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	in, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("扫描接口 %q 不存在", name)
	}
	addrs, err := in.Addrs()
	if err != nil {
		return nil, fmt.Errorf("读取扫描接口 %q: %w", name, err)
	}
	var fallback net.IP
	for _, addr := range addrs {
		var ip net.IP
		switch value := addr.(type) {
		case *net.IPNet:
			ip = value.IP.To4()
		case *net.IPAddr:
			ip = value.IP.To4()
		}
		if ip == nil {
			continue
		}
		if ip.IsPrivate() {
			return append(net.IP(nil), ip...), nil
		}
		if fallback == nil {
			fallback = append(net.IP(nil), ip...)
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("扫描接口 %q 没有 IPv4 地址", name)
}
func linuxGateways() []string {
	f, e := os.Open("/proc/net/route")
	if e != nil {
		return nil
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	s.Scan()
	for s.Scan() {
		p := strings.Fields(s.Text())
		if len(p) < 3 || p[1] != "00000000" {
			continue
		}
		n, e := strconv.ParseUint(p[2], 16, 32)
		if e == nil {
			out = append(out, fmt.Sprintf("%d.%d.%d.%d", byte(n), byte(n>>8), byte(n>>16), byte(n>>24)))
		}
	}
	return out
}
func validateCIDR(v string) (*net.IPNet, int, error) {
	ip, n, e := net.ParseCIDR(strings.TrimSpace(v))
	if e != nil || ip.To4() == nil {
		return nil, 0, fmt.Errorf("无效 IPv4 CIDR")
	}
	if !ip.IsPrivate() {
		return nil, 0, fmt.Errorf("默认仅允许 RFC 1918 私有地址")
	}
	ones, bits := n.Mask.Size()
	count := 1 << (bits - ones)
	if count > 1024 {
		return nil, 0, fmt.Errorf("扫描范围含 %d 个地址，超过 1024 上限", count)
	}
	if count >= 4 {
		count -= 2 // network and broadcast addresses are never probe targets
	}
	return n, count, nil
}
func normalizeMAC(v string) (string, error) {
	h := strings.NewReplacer(":", "", "-", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(v)))
	if len(h) != 12 {
		return "", fmt.Errorf("无效 MAC")
	}
	b := make([]byte, 6)
	for i := range b {
		n, e := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		if e != nil {
			return "", fmt.Errorf("无效 MAC")
		}
		b[i] = byte(n)
	}
	return net.HardwareAddr(b).String(), nil
}
