package app

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) vendorDatabaseStatus(w http.ResponseWriter, _ *http.Request) {
	jsonOut(w, http.StatusOK, s.vendors.Status())
}

func (s *Server) updateVendorDatabase(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := s.vendors.Update(ctx); err != nil {
		fail(w, http.StatusBadGateway, "vendor_database_update_failed", err.Error())
		return
	}
	jsonOut(w, http.StatusOK, s.vendors.Status())
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	values, err := s.store.settings()
	if err != nil {
		fail(w, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	if values["notification_telegram_token"] != "" {
		values["notification_telegram_token"] = "••••••••"
	}
	jsonOut(w, http.StatusOK, values)
}

func (s *Server) testNotification(w http.ResponseWriter, r *http.Request) {
	if s.notifier == nil {
		s.notifier = newNotifier(s.store)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.notifier.SendTest(ctx); err != nil {
		fail(w, http.StatusBadGateway, "notification_failed", err.Error())
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"sent": true})
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var values map[string]any
	if decode(r, &values) != nil {
		fail(w, http.StatusBadRequest, "invalid_request", "设置格式无效")
		return
	}
	if raw, ok := values["scan_cidrs"].(string); ok {
		for _, cidr := range strings.Split(raw, ",") {
			if _, _, err := validateCIDR(cidr); err != nil {
				fail(w, http.StatusBadRequest, "invalid_cidr", err.Error())
				return
			}
		}
	}
	if raw, ok := values["gateway_ip"].(string); ok && raw != "" {
		ip := net.ParseIP(raw)
		if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
			fail(w, http.StatusBadRequest, "invalid_gateway", "主网关必须是私有 IPv4 地址")
			return
		}
	}
	for key, bounds := range map[string][2]int{"automatic_backup_keep": {1, 30}, "history_retention_days": {7, 365}} {
		if raw, ok := values[key].(float64); ok && (raw != float64(int(raw)) || int(raw) < bounds[0] || int(raw) > bounds[1]) {
			fail(w, http.StatusBadRequest, "invalid_setting", "备份保留份数或历史保留天数超出范围")
			return
		}
	}
	if raw, ok := values["automatic_backup_interval"].(string); ok {
		interval, err := time.ParseDuration(raw)
		if err != nil || interval < 6*time.Hour || interval > 7*24*time.Hour {
			fail(w, http.StatusBadRequest, "invalid_setting", "自动备份间隔必须在 6 小时到 7 天之间")
			return
		}
	}
	current, _ := s.store.settings()
	for key, value := range values {
		if !allowedSetting(key) {
			continue
		}
		switch value := value.(type) {
		case string:
			if key == "notification_telegram_token" && value == "••••••••" {
				delete(values, key)
				continue
			}
			current[key] = value
		case bool:
			current[key] = strconv.FormatBool(value)
		case float64:
			if value != float64(int(value)) {
				fail(w, http.StatusBadRequest, "invalid_setting", "数值设置必须是整数")
				return
			}
			current[key] = strconv.Itoa(int(value))
		default:
			fail(w, http.StatusBadRequest, "invalid_setting", "设置值类型无效")
			return
		}
	}
	nextConfig, err := applyStoredSettings(s.scanner.config(), current)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_setting", err.Error())
		return
	}
	if err = s.store.saveSettings(values); err != nil {
		fail(w, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	if gateway, ok := values["gateway_ip"].(string); ok && gateway != "" {
		_ = s.store.ensureCore(gateway)
	}
	oldInterval := s.scanner.config().ScanInterval
	s.scanner.UpdateConfig(nextConfig)
	if nextConfig.ScanInterval != oldInterval {
		s.replaceScanInterval(nextConfig.ScanInterval)
	}
	jsonOut(w, http.StatusOK, map[string]bool{"saved": true})
}

func (s *Server) replaceScanInterval(interval time.Duration) {
	select {
	case s.intervalUpdates <- interval:
		return
	default:
	}
	select {
	case <-s.intervalUpdates:
	default:
	}
	s.intervalUpdates <- interval
}
