package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
