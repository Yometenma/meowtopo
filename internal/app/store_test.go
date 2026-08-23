package app

import (
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
	d, e := s.upsertSeen("192.168.7.10", "aa:bb:cc:dd:ee:ff", "auto", "unknown", 2)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.db.Exec(`UPDATE devices SET user_name='我的 NAS',user_device_type='nas',notes='机柜' WHERE id=?`, d.ID)
	if e != nil {
		t.Fatal(e)
	}
	d, e = s.upsertSeen("192.168.7.20", "aa:bb:cc:dd:ee:ff", "changed", "linux", 3)
	if e != nil {
		t.Fatal(e)
	}
	if d.UserName != "我的 NAS" || d.UserType != "nas" || d.Notes != "机柜" || d.IP != "192.168.7.20" {
		t.Fatalf("manual fields overwritten: %+v", d)
	}
}
func TestOfflineThreshold(t *testing.T) {
	s := testStore(t)
	d, e := s.upsertSeen("10.0.1.5", "", "host", "unknown", 1)
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
