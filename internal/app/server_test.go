package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusEventsReturnsNewestFirstAndHonorsLimit(t *testing.T) {
	s := testServer(t)
	device, err := s.store.createManual("客厅交换机", "switch", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"suspected_offline", "offline"} {
		if _, err = s.store.db.Exec(`INSERT INTO status_events(device_id,event_type,old_status,new_status,created_at) VALUES(?,'status','online',?,?)`, device.ID, status, now()); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	s.statusEvents(rec, httptest.NewRequest(http.MethodGet, "/api/status/events?limit=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var rows []struct {
		DeviceName string `json:"device_name"`
		NewStatus  string `json:"new_status"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DeviceName != "客厅交换机" || rows[0].NewStatus != "offline" {
		t.Fatalf("unexpected events: %+v", rows)
	}
}

func TestTrimStatusEventsKeepsNewest(t *testing.T) {
	store := testStore(t)
	for i := 0; i < 5; i++ {
		if _, err := store.db.Exec(`INSERT INTO status_events(event_type,old_status,new_status,created_at) VALUES('status','online','offline',?)`, now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.trimStatusEvents(3); err != nil {
		t.Fatal(err)
	}
	var count, minID int
	if err := store.db.QueryRow(`SELECT COUNT(*),MIN(id) FROM status_events`).Scan(&count, &minID); err != nil {
		t.Fatal(err)
	}
	if count != 3 || minID != 3 {
		t.Fatalf("count=%d minID=%d", count, minID)
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	store := testStore(t)
	events := newHub()
	base := Config{ScanInterval: 5 * time.Minute, PingTimeout: 800 * time.Millisecond, TCPTimeout: 350 * time.Millisecond, Concurrency: 32, OfflineThreshold: 3, EnablePortScan: true}
	s := &Server{store: store, events: events, intervalUpdates: make(chan time.Duration, 1)}
	s.scanner = &Scanner{store: store, cfg: base, events: events}
	return s
}

func TestPatchSettingsUpdatesRunningScanner(t *testing.T) {
	store := testStore(t)
	base := Config{
		CIDRs:            []string{"192.168.1.0/24"},
		ScanInterval:     5 * time.Minute,
		PingTimeout:      800 * time.Millisecond,
		TCPTimeout:       350 * time.Millisecond,
		Concurrency:      32,
		OfflineThreshold: 3,
		EnablePortScan:   true,
	}
	s := &Server{
		store:           store,
		events:          newHub(),
		intervalUpdates: make(chan time.Duration, 1),
	}
	s.scanner = &Scanner{store: store, cfg: base, events: s.events}

	body := bytes.NewBufferString(`{"scan_interface":"","scan_cidrs":"10.0.1.0/24","scan_interval":"15m","scan_concurrency":64,"ping_timeout":"1s","tcp_timeout":"500ms","offline_threshold":5,"enable_port_scan":false}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/settings", body)
	rec := httptest.NewRecorder()
	s.patchSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	got := s.scanner.config()
	if len(got.CIDRs) != 1 || got.CIDRs[0] != "10.0.1.0/24" || got.ScanInterval != 15*time.Minute || got.Concurrency != 64 || got.PingTimeout != time.Second || got.TCPTimeout != 500*time.Millisecond || got.OfflineThreshold != 5 || got.EnablePortScan {
		t.Fatalf("runtime settings not updated: %+v", got)
	}
	select {
	case interval := <-s.intervalUpdates:
		if interval != 15*time.Minute {
			t.Fatalf("interval update=%s", interval)
		}
	default:
		t.Fatal("scheduler was not notified")
	}
}

func TestDeleteOnlyManualDevice(t *testing.T) {
	s := testServer(t)
	manual, err := s.store.createManual("客厅交换机", "switch", "")
	if err != nil {
		t.Fatal(err)
	}
	automatic, err := s.store.upsertSeen(Discovery{IP: "192.168.8.20", Type: "unknown"})
	if err != nil {
		t.Fatal(err)
	}

	request := func(id int64) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/devices/id", nil)
		req.SetPathValue("id", fmt.Sprint(id))
		rec := httptest.NewRecorder()
		s.deleteDevice(rec, req)
		return rec
	}
	if rec := request(automatic.ID); rec.Code != http.StatusConflict {
		t.Fatalf("automatic device deletion status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(manual.ID); rec.Code != http.StatusOK {
		t.Fatalf("manual device deletion status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := s.store.device(manual.ID); err == nil {
		t.Fatal("manual device still exists")
	}
}

func TestConnectionCanBeCleared(t *testing.T) {
	s := testServer(t)
	parent, _ := s.store.createManual("交换机", "switch", "")
	child, _ := s.store.createManual("AP", "ap", "")
	_, err := s.store.db.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,source_type,confidence,user_confirmed,created_at,updated_at)VALUES(?,?,'ethernet','manual',1,1,?,?)`, parent.ID, child.ID, now(), now())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/devices/id/connection", bytes.NewBufferString(`{"parent_id":0,"connection_type":"unknown","port_label":""}`))
	req.SetPathValue("id", fmt.Sprint(child.ID))
	rec := httptest.NewRecorder()
	s.connection(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	connections, _ := s.store.connections()
	if len(connections) != 0 {
		t.Fatalf("connection was not removed: %+v", connections)
	}
}

func TestBatchDeviceActions(t *testing.T) {
	s := testServer(t)
	parent, _ := s.store.createManual("核心交换机", "switch", "")
	a, _ := s.store.createManual("设备 A", "unknown", "")
	b, _ := s.store.createManual("设备 B", "unknown", "")
	if _, err := s.store.db.Exec(`UPDATE devices SET is_new=1 WHERE id IN (?,?)`, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/devices/batch", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		s.batchDevices(rec, req)
		return rec
	}
	ids := fmt.Sprintf(`[%d,%d]`, a.ID, b.ID)
	if rec := call(fmt.Sprintf(`{"ids":%s,"action":"hide"}`, ids)); rec.Code != http.StatusOK {
		t.Fatalf("hide status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, id := range []int64{a.ID, b.ID} {
		d, _ := s.store.device(id)
		if !d.IsHidden {
			t.Fatalf("device %d was not hidden", id)
		}
	}
	if rec := call(fmt.Sprintf(`{"ids":%s,"action":"clear_new"}`, ids)); rec.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := call(fmt.Sprintf(`{"ids":%s,"action":"set_parent","parent_id":%d,"connection_type":"ethernet"}`, ids, parent.ID)); rec.Code != http.StatusOK {
		t.Fatalf("parent status=%d body=%s", rec.Code, rec.Body.String())
	}
	connections, _ := s.store.connections()
	if len(connections) != 2 {
		t.Fatalf("connections=%+v", connections)
	}
	if rec := call(fmt.Sprintf(`{"ids":%s,"action":"set_parent","parent_id":%d}`, ids, a.ID)); rec.Code != http.StatusBadRequest {
		t.Fatalf("self-parent status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeviceHistorySummary(t *testing.T) {
	s := testServer(t)
	d, err := s.store.upsertSeen(Discovery{IP: "192.168.9.10", Type: "unknown", Latency: 10, ProbeMethod: "icmp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.db.Exec(`INSERT INTO device_samples(device_id,checked_at,status,latency_ms,probe_method) VALUES(?,?,'offline',0,'')`, d.ID, now()); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/devices/id/history?hours=24", nil)
	req.SetPathValue("id", fmt.Sprint(d.ID))
	rec := httptest.NewRecorder()
	s.deviceHistory(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"uptime_percent":50`) || !strings.Contains(rec.Body.String(), `"average_latency_ms":10`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStaticCacheControl(t *testing.T) {
	for _, path := range []string{"/", "/index.html", "/app.js", "/features.js", "/style.css"} {
		if got := staticCacheControl(path); !strings.Contains(got, "no-store") {
			t.Fatalf("staticCacheControl(%q)=%q, want no-store", path, got)
		}
	}
	if got := staticCacheControl("/assets/devices/topo-router.png"); !strings.Contains(got, "max-age") {
		t.Fatalf("asset cache control=%q, want max-age", got)
	}
}
