package app

import (
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
