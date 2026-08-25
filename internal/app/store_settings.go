package app

import (
	"encoding/json"
	"strings"
)

func (s *Store) settings() (map[string]string, error) {
	r, e := s.db.Query(`SELECT key,value FROM settings`)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	m := map[string]string{}
	for r.Next() {
		var k, v string
		if e = r.Scan(&k, &v); e != nil {
			return nil, e
		}
		m[k] = v
	}
	return m, r.Err()
}
func (s *Store) saveSettings(m map[string]any) error {
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for k, v := range m {
		if !allowedSetting(k) {
			continue
		}
		b, _ := json.Marshal(v)
		val := strings.Trim(string(b), `"`)
		if _, e = tx.Exec(`INSERT INTO settings(key,value,updated_at)VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, k, val, now()); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func allowedSetting(k string) bool {
	switch k {
	case "initialized", "scan_interface", "scan_cidrs", "gateway_ip", "scan_interval", "scan_concurrency", "ping_timeout", "tcp_timeout", "offline_threshold", "enable_port_scan", "theme", "label_mode", "hide_offline_days",
		"notification_enabled", "notification_telegram_enabled", "notification_telegram_token", "notification_telegram_chat_id", "notification_webhook_enabled", "notification_webhook_url",
		"notification_new_device", "notification_offline", "notification_online", "notification_scan_error":
		return true
	case "notification_cooldown", "notification_important_only":
		return true
	case "automatic_backup_enabled", "automatic_backup_interval", "automatic_backup_keep", "history_retention_days":
		return true
	}
	return false
}
