package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

	body := bytes.NewBufferString(`{"scan_cidrs":"10.0.1.0/24","scan_interval":"15m","scan_concurrency":64,"ping_timeout":"1s","tcp_timeout":"500ms","offline_threshold":5,"enable_port_scan":false}`)
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
