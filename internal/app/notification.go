package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Notifier struct {
	store        *Store
	client       *http.Client
	telegramBase string
}

type notificationPayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Event   string `json:"event"`
	Source  string `json:"source"`
}

func newNotifier(store *Store) *Notifier {
	return &Notifier{store: store, client: &http.Client{Timeout: 8 * time.Second}, telegramBase: "https://api.telegram.org"}
}

func settingEnabled(settings map[string]string, key string, fallback bool) bool {
	v, ok := settings[key]
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.EqualFold(v, "true")
}

func (n *Notifier) SendTest(ctx context.Context) error {
	return n.send(ctx, "test", "喵拓测试通知", "通知连接成功，MeowTopo 已经可以向这里发送网络动态。")
}

func (n *Notifier) NotifyScan(startedAt string, status ScanStatus) {
	settings, err := n.store.settings()
	if err != nil || !settingEnabled(settings, "notification_enabled", false) {
		return
	}
	lines := []string{}
	type pendingNotification struct {
		deviceID int64
		event    string
	}
	pending := []pendingNotification{}
	cooldown := 15 * time.Minute
	if parsed, parseErr := time.ParseDuration(settings["notification_cooldown"]); parseErr == nil && parsed >= 0 {
		cooldown = parsed
	}
	importantOnly := settingEnabled(settings, "notification_important_only", false)
	if settingEnabled(settings, "notification_new_device", true) {
		rows, err := n.store.db.Query(`SELECT id,COALESCE(NULLIF(user_name,''),NULLIF(auto_hostname,''),NULLIF(current_ip,''),'未命名设备'),current_ip,is_important FROM devices WHERE created_manually=0 AND first_seen_at>=? ORDER BY id`, startedAt)
		if err == nil {
			for rows.Next() {
				var id int64
				var name, ip string
				var important bool
				if rows.Scan(&id, &name, &ip, &important) == nil && (!importantOnly || important) && n.allowNotification(id, "new", cooldown) {
					lines = append(lines, fmt.Sprintf("发现新设备：%s%s", name, formatIP(ip)))
					pending = append(pending, pendingNotification{id, "new"})
				}
			}
			rows.Close()
		}
	}
	rows, err := n.store.db.Query(`SELECT e.device_id,COALESCE(NULLIF(d.user_name,''),NULLIF(d.auto_hostname,''),NULLIF(d.current_ip,''),'已删除设备'),d.current_ip,e.new_status,COALESCE(d.is_important,0) FROM status_events e LEFT JOIN devices d ON d.id=e.device_id WHERE e.created_at>=? ORDER BY e.id`, startedAt)
	if err == nil {
		for rows.Next() {
			var deviceID int64
			var name, ip, newStatus string
			var important bool
			if rows.Scan(&deviceID, &name, &ip, &newStatus, &important) != nil || (importantOnly && !important) {
				continue
			}
			if newStatus == "online" && settingEnabled(settings, "notification_online", true) && n.allowNotification(deviceID, "online", cooldown) {
				lines = append(lines, fmt.Sprintf("设备重新上线：%s%s", name, formatIP(ip)))
				pending = append(pending, pendingNotification{deviceID, "online"})
			}
			if newStatus == "offline" && settingEnabled(settings, "notification_offline", true) && n.allowNotification(deviceID, "offline", cooldown) {
				lines = append(lines, fmt.Sprintf("设备已离线：%s%s", name, formatIP(ip)))
				pending = append(pending, pendingNotification{deviceID, "offline"})
			}
		}
		rows.Close()
	}
	if status.Error != "" && settingEnabled(settings, "notification_scan_error", true) {
		lines = append(lines, "扫描异常："+status.Error)
	}
	if len(lines) == 0 {
		return
	}
	if err := n.send(context.Background(), "scan", "喵拓网络动态", strings.Join(lines, "\n")); err == nil {
		for _, item := range pending {
			_, _ = n.store.db.Exec(`INSERT INTO notification_state(device_id,event_type,last_sent_at) VALUES(?,?,?) ON CONFLICT(device_id,event_type) DO UPDATE SET last_sent_at=excluded.last_sent_at`, item.deviceID, item.event, now())
		}
	}
}

func (n *Notifier) allowNotification(deviceID int64, event string, cooldown time.Duration) bool {
	if cooldown > 0 {
		var last string
		if err := n.store.db.QueryRow(`SELECT last_sent_at FROM notification_state WHERE device_id=? AND event_type=?`, deviceID, event).Scan(&last); err == nil {
			if sentAt, parseErr := time.Parse(time.RFC3339, last); parseErr == nil && time.Since(sentAt) < cooldown {
				return false
			}
		}
	}
	return true
}

func formatIP(ip string) string {
	if ip == "" {
		return ""
	}
	return "（" + ip + "）"
}

func (n *Notifier) send(ctx context.Context, event, title, message string) error {
	settings, err := n.store.settings()
	if err != nil {
		return err
	}
	if !settingEnabled(settings, "notification_enabled", false) {
		return fmt.Errorf("请先启用外部通知")
	}
	sent := 0
	errors := []string{}
	if settingEnabled(settings, "notification_telegram_enabled", false) {
		token, chatID := strings.TrimSpace(settings["notification_telegram_token"]), strings.TrimSpace(settings["notification_telegram_chat_id"])
		if token == "" || chatID == "" {
			errors = append(errors, "Telegram 的 Token 或 Chat ID 未填写")
		} else if err := n.sendTelegram(ctx, token, chatID, title+"\n\n"+message); err != nil {
			errors = append(errors, "Telegram："+err.Error())
		} else {
			sent++
		}
	}
	if settingEnabled(settings, "notification_webhook_enabled", false) {
		endpoint := strings.TrimSpace(settings["notification_webhook_url"])
		if err := validateHTTPURL(endpoint); err != nil {
			errors = append(errors, "Webhook："+err.Error())
		} else if err := n.sendWebhook(ctx, endpoint, notificationPayload{Title: title, Message: message, Event: event, Source: "MeowTopo"}); err != nil {
			errors = append(errors, "Webhook："+err.Error())
		} else {
			sent++
		}
	}
	if sent == 0 && len(errors) == 0 {
		return fmt.Errorf("请至少启用一种通知方式")
	}
	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "；"))
	}
	return nil
}

func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("地址必须是完整的 http 或 https URL")
	}
	return nil
}

func (n *Notifier) sendTelegram(ctx context.Context, token, chatID, text string) error {
	form := url.Values{"chat_id": {chatID}, "text": {text}, "disable_web_page_preview": {"true"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(n.telegramBase, "/")+"/bot"+token+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return n.do(req)
}

func (n *Notifier) sendWebhook(ctx context.Context, endpoint string, payload notificationPayload) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return n.do(req)
}

func (n *Notifier) do(req *http.Request) error {
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("对方返回 %d%s", resp.StatusCode, responseSuffix(body))
	}
	return nil
}

func responseSuffix(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	return "：" + text
}
