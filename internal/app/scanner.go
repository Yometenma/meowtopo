package app

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ScanStatus struct {
	Running    bool   `json:"running"`
	RunID      int64  `json:"run_id"`
	CIDR       string `json:"cidr"`
	Total      int    `json:"total"`
	Scanned    int    `json:"scanned"`
	Found      int    `json:"found"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	Error      string `json:"error,omitempty"`
}
type ProbeResult struct {
	Alive     bool
	Latency   float64
	Method    string
	OpenPorts []int
}
type Scanner struct {
	mu       sync.RWMutex
	status   ScanStatus
	store    *Store
	cfg      Config
	events   *EventHub
	notifier *Notifier
}

func (s *Scanner) Status() ScanStatus { s.mu.RLock(); defer s.mu.RUnlock(); return s.status }
func (s *Scanner) UpdateConfig(cfg Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}
func (s *Scanner) config() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}
func (s *Scanner) Start(cidr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Running {
		return fmt.Errorf("扫描任务已在运行")
	}
	n, total, e := validateCIDR(cidr)
	if e != nil {
		return e
	}
	r, e := s.store.db.Exec(`INSERT INTO scan_runs(started_at,status,cidrs,total_addresses)VALUES(?,'running',?,?)`, now(), cidr, total)
	if e != nil {
		return e
	}
	id, _ := r.LastInsertId()
	s.status = ScanStatus{Running: true, RunID: id, CIDR: cidr, Total: total, StartedAt: now()}
	go s.run(n)
	s.events.Emit("scan_started", s.status)
	return nil
}
func (s *Scanner) run(n *net.IPNet) {
	cfg := s.config()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.Status().Total)*cfg.TCPTimeout+2*time.Minute)
	defer cancel()
	jobs := make(chan string)
	seen := map[string]bool{}
	var seenMu sync.Mutex
	var wg sync.WaitGroup
	workers := cfg.Concurrency
	if workers > s.Status().Total {
		workers = s.Status().Total
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				result := probeTarget(ctx, ip, cfg)
				mac := arpMAC(ip)
				result = applyARPResult(result, mac)
				if result.Alive {
					seenMu.Lock()
					seen[ip] = true
					seenMu.Unlock()
					host := ""
					if names, _ := net.LookupAddr(ip); len(names) > 0 {
						host = names[0]
					}
					typ, source, confidence := identifyType(host, result.OpenPorts)
					d, _ := s.store.upsertSeen(Discovery{IP: ip, MAC: mac, Hostname: host, Type: typ, Latency: result.Latency, ProbeMethod: result.Method, OpenPorts: result.OpenPorts, TypeSource: source, TypeConfidence: confidence})
					s.events.Emit("device_seen", d)
				}
				s.mu.Lock()
				s.status.Scanned++
				if result.Alive {
					s.status.Found++
				}
				st := s.status
				s.mu.Unlock()
				if st.Scanned%10 == 0 || st.Scanned == st.Total {
					s.events.Emit("scan_progress", st)
				}
			}
		}()
	}
sendLoop:
	for ip, broadcast := firstIP(n), lastIP(n); n.Contains(ip) && !ip.Equal(broadcast); ip = nextIP(ip) {
		select {
		case jobs <- ip.String():
		case <-ctx.Done():
			break sendLoop
		}
	}
	close(jobs)
	wg.Wait()
	for ip, mac := range arpNeighbors() {
		parsed := net.ParseIP(ip)
		if parsed == nil || !n.Contains(parsed) || seen[ip] {
			continue
		}
		seen[ip] = true
		d, err := s.store.upsertSeen(Discovery{IP: ip, MAC: mac, Type: "unknown", ProbeMethod: "arp"})
		if err != nil {
			continue
		}
		s.mu.Lock()
		s.status.Found++
		s.mu.Unlock()
		s.events.Emit("device_seen", d)
	}
	_ = s.store.markMisses(seen, cfg.OfflineThreshold)
	if set, _ := s.store.settings(); set["gateway_ip"] != "" {
		_ = s.store.ensureCore(set["gateway_ip"])
	}
	s.mu.Lock()
	s.status.Running = false
	s.status.FinishedAt = now()
	if ctx.Err() != nil {
		s.status.Error = ctx.Err().Error()
	}
	st := s.status
	s.mu.Unlock()
	state := "completed"
	if st.Error != "" {
		state = "failed"
	}
	_, _ = s.store.db.Exec(`UPDATE scan_runs SET finished_at=?,status=?,scanned_addresses=?,found_devices=?,error_summary=? WHERE id=?`, st.FinishedAt, state, st.Scanned, st.Found, st.Error, st.RunID)
	s.events.Emit("scan_completed", st)
	if s.notifier != nil {
		go s.notifier.NotifyScan(st.StartedAt, st)
	}
}
func applyARPResult(result ProbeResult, mac string) ProbeResult {
	if !result.Alive && mac != "" {
		result.Alive = true
		result.Method = "arp"
	}
	return result
}
func (s *Scanner) probe(ctx context.Context, ip string) ProbeResult {
	cfg := s.config()
	return probeTarget(ctx, ip, cfg)
}
func probeTarget(ctx context.Context, ip string, cfg Config) ProbeResult {
	sourceIP, err := interfaceIPv4(cfg.Interface)
	if err != nil {
		return ProbeResult{}
	}
	result := ProbeResult{}
	if ok, latency := icmpProbe(ip, sourceIP, cfg.PingTimeout); ok {
		result.Alive = true
		result.Latency = latency
		result.Method = "icmp"
	}
	if !cfg.EnablePortScan {
		return result
	}
	ports := probePorts(cfg.EnablePortScan)
	for _, p := range ports {
		start := time.Now()
		d := net.Dialer{Timeout: cfg.TCPTimeout}
		if sourceIP != nil {
			d.LocalAddr = &net.TCPAddr{IP: sourceIP}
		}
		c, e := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ip, p))
		if e == nil {
			c.Close()
			result.OpenPorts = append(result.OpenPorts, p)
			if !result.Alive {
				result.Alive = true
				result.Latency = float64(time.Since(start).Microseconds()) / 1000
				result.Method = "tcp_connect"
			} else if result.Method == "icmp" {
				result.Method = "icmp+tcp_connect"
			}
		}
	}
	return result
}
func probePorts(enabled bool) []int {
	if !enabled {
		return nil
	}
	return []int{22, 53, 80, 443, 445, 554, 8123, 32400}
}
func icmpProbe(target string, sourceIP net.IP, timeout time.Duration) (bool, float64) {
	addr := net.ParseIP(target)
	if addr == nil {
		return false, 0
	}
	var source *net.IPAddr
	if sourceIP != nil {
		source = &net.IPAddr{IP: sourceIP}
	}
	c, err := net.DialIP("ip4:icmp", source, &net.IPAddr{IP: addr})
	if err != nil {
		return false, 0
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	id := os.Getpid() & 0xffff
	msg := []byte{8, 0, 0, 0, byte(id >> 8), byte(id), 0, 1, 'M', 'e', 'o', 'w', 'T', 'o', 'p', 'o'}
	cs := icmpChecksum(msg)
	msg[2], msg[3] = byte(cs>>8), byte(cs)
	start := time.Now()
	if _, err = c.Write(msg); err != nil {
		return false, 0
	}
	buf := make([]byte, 1500)
	for {
		n, _, err := c.ReadFromIP(buf)
		if err != nil {
			return false, 0
		}
		if n >= 8 && buf[0] == 0 && int(buf[4])<<8|int(buf[5]) == id {
			return true, float64(time.Since(start).Microseconds()) / 1000
		}
	}
}
func icmpChecksum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(b[0])<<8 | uint32(b[1])
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}
func firstIP(n *net.IPNet) net.IP { ip := append(net.IP(nil), n.IP.To4()...); return nextIP(ip) }
func nextIP(ip net.IP) net.IP {
	out := append(net.IP(nil), ip...)
	for i := len(out) - 1; i >= 0; i-- {
		out[i]++
		if out[i] != 0 {
			break
		}
	}
	return out
}
func lastIP(n *net.IPNet) net.IP {
	ip := append(net.IP(nil), n.IP.To4()...)
	for i := range ip {
		ip[i] |= ^n.Mask[i]
	}
	return ip
}
func arpMAC(ip string) string {
	if runtime.GOOS != "linux" {
		return ""
	}
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Scan()
	for s.Scan() {
		if mac := parseARPLine(s.Text(), ip); mac != "" {
			return mac
		}
	}
	return ""
}
func arpNeighbors() map[string]string {
	out := map[string]string{}
	if runtime.GOOS == "linux" {
		file, err := os.Open("/proc/net/arp")
		if err != nil {
			return out
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Scan()
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) > 0 {
				if mac := parseARPLine(scanner.Text(), fields[0]); mac != "" {
					out[fields[0]] = mac
				}
			}
		}
		return out
	}
	if runtime.GOOS == "windows" {
		data, err := exec.Command("arp", "-a").Output()
		if err != nil {
			return out
		}
		return parseWindowsARP(string(data))
	}
	return out
}
func parseWindowsARP(data string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || net.ParseIP(fields[0]) == nil {
			continue
		}
		mac, err := normalizeMAC(fields[1])
		if err == nil && mac != "00:00:00:00:00:00" && mac != "ff:ff:ff:ff:ff:ff" {
			out[fields[0]] = mac
		}
	}
	return out
}
func parseARPLine(line, ip string) string {
	fields := strings.Fields(line)
	if len(fields) < 4 || fields[0] != ip {
		return ""
	}
	flags, err := strconv.ParseUint(fields[2], 0, 32)
	if err != nil || flags&0x2 == 0 {
		return ""
	}
	mac, err := normalizeMAC(fields[3])
	if err != nil || mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" {
		return ""
	}
	return mac
}
func identifyType(host string, ports []int) (string, string, float64) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	hostRules := []struct {
		kind       string
		confidence float64
		words      []string
	}{
		{"nas", .92, []string{"synology", "qnap", "truenas", "openmediavault", "-nas", "nas-"}},
		{"router", .9, []string{"openwrt", "opnsense", "pfsense", "router", "gateway"}},
		{"camera", .88, []string{"ipcam", "camera", "webcam", "nvr", "dvr"}},
		{"printer", .86, []string{"printer", "laserjet", "deskjet", "officejet"}},
		{"iot", .84, []string{"homeassistant", "home-assistant", "esphome", "shelly"}},
		{"linux", .78, []string{"proxmox", "pve", "esxi", "server", "ubuntu", "debian"}},
		{"phone", .72, []string{"iphone", "android", "pixel", "phone"}},
		{"tv", .7, []string{"smarttv", "androidtv", "appletv", "chromecast"}},
	}
	for _, rule := range hostRules {
		for _, word := range rule.words {
			if strings.Contains(host, word) {
				return rule.kind, "hostname", rule.confidence
			}
		}
	}
	open := map[int]bool{}
	for _, port := range ports {
		open[port] = true
	}
	switch {
	case open[8123]:
		return "iot", "ports", .78
	case open[554]:
		return "camera", "ports", .72
	case open[32400]:
		return "nas", "ports", .62
	case open[53] && (open[80] || open[443]):
		return "router", "ports", .6
	case open[445]:
		return "nas", "ports", .55
	case open[22]:
		return "linux", "ports", .45
	default:
		return "unknown", "", 0
	}
}
