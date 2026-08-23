package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr, DataDir, Interface, GatewayIP, LogLevel string
	CIDRs                                             []string
	ScanInterval, PingTimeout, TCPTimeout             time.Duration
	Concurrency, OfflineThreshold                     int
	EnablePortScan                                    bool
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
func envInt(key string, fallback int) int {
	v, e := strconv.Atoi(env(key, ""))
	if e == nil && v > 0 {
		return v
	}
	return fallback
}
func envBool(key string, fallback bool) bool {
	v, e := strconv.ParseBool(env(key, ""))
	if e == nil {
		return v
	}
	return fallback
}
func envDuration(key string, fallback time.Duration) time.Duration {
	v, e := time.ParseDuration(env(key, ""))
	if e == nil && v > 0 {
		return v
	}
	return fallback
}

func loadConfig() Config {
	c := Config{HTTPAddr: env("MEOWTOPO_HTTP_ADDR", "127.0.0.1:8088"), DataDir: env("MEOWTOPO_DATA_DIR", "./data"), Interface: env("MEOWTOPO_SCAN_INTERFACE", ""), GatewayIP: env("MEOWTOPO_GATEWAY_IP", ""), LogLevel: env("MEOWTOPO_LOG_LEVEL", "info"), ScanInterval: envDuration("MEOWTOPO_SCAN_INTERVAL", 5*time.Minute), PingTimeout: envDuration("MEOWTOPO_PING_TIMEOUT", 800*time.Millisecond), TCPTimeout: envDuration("MEOWTOPO_TCP_TIMEOUT", 350*time.Millisecond), Concurrency: envInt("MEOWTOPO_SCAN_CONCURRENCY", 32), OfflineThreshold: envInt("MEOWTOPO_OFFLINE_THRESHOLD", 3), EnablePortScan: envBool("MEOWTOPO_ENABLE_PORT_SCAN", true)}
	for _, v := range strings.Split(env("MEOWTOPO_SCAN_CIDRS", ""), ",") {
		if v = strings.TrimSpace(v); v != "" {
			c.CIDRs = append(c.CIDRs, v)
		}
	}
	if c.Concurrency > 128 {
		c.Concurrency = 128
	}
	if c.OfflineThreshold > 20 {
		c.OfflineThreshold = 20
	}
	return c
}

func applyStoredSettings(c Config, settings map[string]string) (Config, error) {
	if v, ok := settings["scan_interface"]; ok {
		c.Interface = strings.TrimSpace(v)
	}
	if v := strings.TrimSpace(settings["scan_cidrs"]); v != "" {
		c.CIDRs = nil
		for _, raw := range strings.Split(v, ",") {
			raw = strings.TrimSpace(raw)
			if _, _, err := validateCIDR(raw); err != nil {
				return c, err
			}
			c.CIDRs = append(c.CIDRs, raw)
		}
	}
	if v := strings.TrimSpace(settings["scan_concurrency"]); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 128 {
			return c, fmt.Errorf("扫描并发数量必须在 1 到 128 之间")
		}
		c.Concurrency = n
	}
	if v := strings.TrimSpace(settings["offline_threshold"]); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2 || n > 20 {
			return c, fmt.Errorf("离线判定次数必须在 2 到 20 之间")
		}
		c.OfflineThreshold = n
	}
	for key, target := range map[string]*time.Duration{
		"scan_interval": &c.ScanInterval,
		"ping_timeout":  &c.PingTimeout,
		"tcp_timeout":   &c.TCPTimeout,
	} {
		if v := strings.TrimSpace(settings[key]); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				return c, fmt.Errorf("%s 必须是有效的正数时长", key)
			}
			*target = d
		}
	}
	if v := strings.TrimSpace(settings["enable_port_scan"]); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("端口探测开关无效")
		}
		c.EnablePortScan = b
	}
	if _, err := interfaceIPv4(c.Interface); err != nil {
		return c, err
	}
	return c, nil
}
