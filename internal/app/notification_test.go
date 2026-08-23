package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookNotification(t *testing.T) {
	var got notificationPayload
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()

	store := testStore(t)
	if err := store.saveSettings(map[string]any{
		"notification_enabled":         true,
		"notification_webhook_enabled": true,
		"notification_webhook_url":     receiver.URL,
	}); err != nil {
		t.Fatal(err)
	}
	if err := newNotifier(store).SendTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.Event != "test" || got.Source != "MeowTopo" || !strings.Contains(got.Message, "通知连接成功") {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestTelegramNotification(t *testing.T) {
	var path, body string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		body = r.Form.Get("chat_id") + "|" + r.Form.Get("text")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer receiver.Close()

	store := testStore(t)
	if err := store.saveSettings(map[string]any{
		"notification_enabled":          true,
		"notification_telegram_enabled": true,
		"notification_telegram_token":   "fake-token",
		"notification_telegram_chat_id": "-100123",
	}); err != nil {
		t.Fatal(err)
	}
	notifier := newNotifier(store)
	notifier.telegramBase = receiver.URL
	if err := notifier.SendTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/botfake-token/sendMessage" || !strings.Contains(body, "-100123|喵拓测试通知") {
		t.Fatalf("path=%q body=%q", path, body)
	}
}

func TestNotificationTokenIsMasked(t *testing.T) {
	s := testServer(t)
	if err := s.store.saveSettings(map[string]any{"notification_telegram_token": "secret-token"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.getSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if strings.Contains(rec.Body.String(), "secret-token") || !strings.Contains(rec.Body.String(), "••••••••") {
		t.Fatalf("token was not masked: %s", rec.Body.String())
	}
}

func TestInvalidWebhookURLIsRejected(t *testing.T) {
	store := testStore(t)
	if err := store.saveSettings(map[string]any{
		"notification_enabled":         true,
		"notification_webhook_enabled": true,
		"notification_webhook_url":     "file:///tmp/message",
	}); err != nil {
		t.Fatal(err)
	}
	if err := newNotifier(store).SendTest(context.Background()); err == nil {
		t.Fatal("invalid webhook URL was accepted")
	}
}

func TestScanNotificationCombinesDeviceChanges(t *testing.T) {
	messages := make(chan string, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload notificationPayload
		_ = json.NewDecoder(r.Body).Decode(&payload)
		messages <- payload.Message
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	store := testStore(t)
	if err := store.saveSettings(map[string]any{
		"notification_enabled":         true,
		"notification_webhook_enabled": true,
		"notification_webhook_url":     receiver.URL,
		"notification_new_device":      true,
		"notification_offline":         true,
	}); err != nil {
		t.Fatal(err)
	}
	started := now()
	newDevice, err := store.upsertSeen(Discovery{IP: "192.168.50.20", Hostname: "living-room-tv", Type: "tv"})
	if err != nil {
		t.Fatal(err)
	}
	offline, err := store.upsertSeen(Discovery{IP: "192.168.50.30", Hostname: "home-server", Type: "nas"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`UPDATE devices SET first_seen_at='2020-01-01T00:00:00Z',status='offline' WHERE id=?`, offline.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`INSERT INTO status_events(device_id,event_type,old_status,new_status,created_at) VALUES(?,'status','online','offline',?)`, offline.ID, now()); err != nil {
		t.Fatal(err)
	}
	newNotifier(store).NotifyScan(started, ScanStatus{})
	message := <-messages
	if !strings.Contains(message, newDevice.AutoHostname) || !strings.Contains(message, "发现新设备") || !strings.Contains(message, "设备已离线") || !strings.Contains(message, offline.AutoHostname) {
		t.Fatalf("unexpected combined message: %s", message)
	}
}
