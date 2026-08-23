package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenanceRemovesExpiredHistory(t *testing.T) {
	s := testServer(t)
	s.cfg.DataDir = filepath.Dir(s.store.path)
	device, err := s.store.createManual("测试设备", "computer", "")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	for _, stamp := range []string{old, recent} {
		if _, err = s.store.db.Exec(`INSERT INTO device_samples(device_id,checked_at,status) VALUES(?,?,'online')`, device.ID, stamp); err != nil {
			t.Fatal(err)
		}
		if _, err = s.store.db.Exec(`INSERT INTO status_events(device_id,event_type,created_at) VALUES(?,'status',?)`, device.ID, stamp); err != nil {
			t.Fatal(err)
		}
		if _, err = s.store.db.Exec(`INSERT INTO scan_runs(started_at,finished_at,status,cidrs) VALUES(?,?,'completed','192.168.1.0/24')`, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.store.saveSettings(map[string]any{"history_retention_days": 30}); err != nil {
		t.Fatal(err)
	}
	s.runMaintenance(time.Now())
	for _, table := range []string{"device_samples", "status_events", "scan_runs"} {
		var count int
		if err = s.store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s count=%d, want 1", table, count)
		}
	}
}

func TestAutomaticBackupContainsDatabase(t *testing.T) {
	s := testServer(t)
	s.cfg.DataDir = t.TempDir()
	info, err := s.makeAutomaticBackup(time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.backupDir(), info.Name)
	if stat, statErr := os.Stat(path); statErr != nil || stat.Size() == 0 {
		t.Fatalf("backup missing or empty: %v", statErr)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 1 || reader.File[0].Name != "meowtopo.db" {
		t.Fatalf("unexpected backup content: %+v", reader.File)
	}
}
