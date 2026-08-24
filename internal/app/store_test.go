package app

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, e := openStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}
func TestManualFieldsSurviveScan(t *testing.T) {
	s := testStore(t)
	d, e := s.upsertSeen(Discovery{IP: "192.168.7.10", MAC: "aa:bb:cc:dd:ee:ff", Hostname: "auto", Type: "unknown", Latency: 2})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.db.Exec(`UPDATE devices SET user_name='我的 NAS',user_device_type='nas',notes='机柜' WHERE id=?`, d.ID)
	if e != nil {
		t.Fatal(e)
	}
	d, e = s.upsertSeen(Discovery{IP: "192.168.7.20", MAC: "aa:bb:cc:dd:ee:ff", Hostname: "changed", Type: "linux", Latency: 3, ProbeMethod: "icmp", OpenPorts: []int{22}, TypeSource: "ports", TypeConfidence: .45, TypeEvidence: []string{"开放端口：22（SSH，弱线索） → linux"}})
	if e != nil {
		t.Fatal(e)
	}
	if d.UserName != "我的 NAS" || d.UserType != "nas" || d.Notes != "机柜" || d.IP != "192.168.7.20" {
		t.Fatalf("manual fields overwritten: %+v", d)
	}
	if d.AutoType != "linux" || d.ProbeMethod != "icmp" || len(d.OpenPorts) != 1 || d.OpenPorts[0] != 22 || d.TypeSource != "ports" || d.TypeConfidence != .45 {
		t.Fatalf("discovery metadata missing: %+v", d)
	}
	if len(d.TypeEvidence) != 1 {
		t.Fatalf("identification evidence missing: %+v", d)
	}
	d, e = s.upsertSeen(Discovery{IP: "192.168.7.20", MAC: "aa:bb:cc:dd:ee:ff", Hostname: "changed", Type: "unknown", Latency: 4, ProbeMethod: "icmp", TypeConfidence: 0})
	if e != nil {
		t.Fatal(e)
	}
	if d.AutoType != "linux" || d.TypeSource != "ports" || d.TypeConfidence != .45 {
		t.Fatalf("lower-confidence scan downgraded identification: %+v", d)
	}
}
func TestOfflineThreshold(t *testing.T) {
	s := testStore(t)
	d, e := s.upsertSeen(Discovery{IP: "10.0.1.5", Hostname: "host", Type: "unknown", Latency: 1})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.markMisses(map[string]bool{}, 3); e != nil {
		t.Fatal(e)
	}
	d, _ = s.device(d.ID)
	if d.Status == "offline" {
		t.Fatal("single failure caused offline")
	}
	s.markMisses(map[string]bool{}, 3)
	s.markMisses(map[string]bool{}, 3)
	d, _ = s.device(d.ID)
	if d.Status != "offline" {
		t.Fatalf("status=%s", d.Status)
	}
	var samples, offlineSamples int
	if err := s.db.QueryRow(`SELECT COUNT(*),SUM(status='offline') FROM device_samples WHERE device_id=?`, d.ID).Scan(&samples, &offlineSamples); err != nil {
		t.Fatal(err)
	}
	if samples != 4 || offlineSamples != 1 {
		t.Fatalf("history samples=%d offline=%d", samples, offlineSamples)
	}
}

func TestImportantDeviceSurvivesDiscovery(t *testing.T) {
	s := testStore(t)
	d, err := s.upsertSeen(Discovery{IP: "192.168.8.8", Type: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE devices SET is_important=1 WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	d, err = s.upsertSeen(Discovery{IP: "192.168.8.8", Type: "tv", TypeConfidence: .7})
	if err != nil || !d.Important {
		t.Fatalf("important flag lost: %+v err=%v", d, err)
	}
}

func TestEnsureCoreReusesScannedGatewayAndRepairsDuplicates(t *testing.T) {
	s := testStore(t)
	if err := s.ensureCore("192.168.1.1"); err != nil {
		t.Fatal(err)
	}
	gateway, err := s.upsertSeen(Discovery{IP: "192.168.1.1", MAC: "aa:bb:cc:dd:ee:01", Hostname: "router", Type: "router", ProbeMethod: "arp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO devices(stable_key,current_ip,user_name,user_device_type,first_seen_at,status,is_new,created_at,updated_at) VALUES('ip:192.168.1.1','192.168.1.1','重复网关','gateway',?,'unknown',0,?,?)`, now(), now(), now()); err != nil {
		t.Fatal(err)
	}
	if err = s.ensureCore("192.168.1.1"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE current_ip='192.168.1.1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("gateway count=%d", count)
	}
	kept, err := s.device(gateway.ID)
	if err != nil {
		t.Fatalf("scanned gateway was not preserved: %v", err)
	}
	if kept.MAC != "aa:bb:cc:dd:ee:01" || kept.UserType != "gateway" {
		t.Fatalf("unexpected gateway: %+v", kept)
	}
	var links int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM connections c JOIN devices source ON source.id=c.source_device_id WHERE source.stable_key='virtual:internet' AND c.target_device_id=?`, kept.ID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("internet to gateway links=%d", links)
	}
}
func TestPositionConnectionAndMigration(t *testing.T) {
	s := testStore(t)
	a, err := s.createManual("交换机", "switch", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.createManual("AP", "ap", "")
	if err != nil {
		t.Fatal(err)
	}
	_, e := s.db.Exec(`INSERT INTO node_positions(device_id,x,y,locked,updated_at)VALUES(?,?,?,?,?)`, a.ID, 12.5, 33.0, 1, now())
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.db.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,source_type,confidence,user_confirmed,created_at,updated_at)VALUES(?,?,'ethernet','manual',1,1,?,?)`, a.ID, b.ID, now(), now())
	if e != nil {
		t.Fatal(e)
	}
	got, _ := s.device(a.ID)
	if got.X != 12.5 || !got.Locked {
		t.Fatalf("position missing %+v", got)
	}
	cs, _ := s.connections()
	if len(cs) != 1 || !cs[0].Confirmed {
		t.Fatalf("connection missing %+v", cs)
	}
	_ = time.Second
}

func TestLegacyDeviceTableMigration(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "meowtopo.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE devices(id INTEGER PRIMARY KEY AUTOINCREMENT,stable_key TEXT NOT NULL UNIQUE,mac_address TEXT DEFAULT '',current_ip TEXT DEFAULT '',auto_hostname TEXT DEFAULT '',user_name TEXT DEFAULT '',vendor TEXT DEFAULT '',auto_device_type TEXT DEFAULT 'unknown',user_device_type TEXT DEFAULT '',icon TEXT DEFAULT '',notes TEXT DEFAULT '',first_seen_at TEXT NOT NULL,last_seen_at TEXT DEFAULT '',last_checked_at TEXT DEFAULT '',status TEXT NOT NULL DEFAULT 'unknown',ping_latency_ms REAL DEFAULT 0,consecutive_successes INTEGER DEFAULT 0,consecutive_failures INTEGER DEFAULT 0,is_new INTEGER DEFAULT 1,is_hidden INTEGER DEFAULT 0,is_ignored INTEGER DEFAULT 0,always_show INTEGER DEFAULT 0,created_manually INTEGER DEFAULT 0,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	store, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.db.Close()
	rows, err := store.db.Query(`PRAGMA table_info(devices)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]bool{"probe_method": false, "open_ports": false, "identification_source": false, "identification_confidence": false, "is_important": false, "presence_mode": false}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("migration did not add %s", name)
		}
	}
}

func TestOccasionalDeviceWaitsLongerBeforeOffline(t *testing.T) {
	s := testStore(t)
	d, err := s.upsertSeen(Discovery{IP: "192.168.44.20", Type: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE devices SET presence_mode='occasional' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 11; i++ {
		if err = s.markMisses(map[string]bool{}, 3); err != nil {
			t.Fatal(err)
		}
	}
	d, _ = s.device(d.ID)
	if d.Status == "offline" {
		t.Fatal("occasional device went offline too early")
	}
	if err = s.markMisses(map[string]bool{}, 3); err != nil {
		t.Fatal(err)
	}
	d, _ = s.device(d.ID)
	if d.Status != "offline" {
		t.Fatalf("status=%s", d.Status)
	}
}

func TestDeviceReportsRecentStatusInstability(t *testing.T) {
	s := testStore(t)
	d, err := s.upsertSeen(Discovery{IP: "192.168.55.8", Type: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = s.db.Exec(`INSERT INTO status_events(device_id,event_type,old_status,new_status,created_at) VALUES(?,'status','online','offline',?)`, d.ID, now()); err != nil {
			t.Fatal(err)
		}
	}
	d, err = s.device(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Flapping || d.StatusChanges != 3 {
		t.Fatalf("instability not reported: %+v", d)
	}
}
