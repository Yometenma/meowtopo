package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

var manualSequence atomic.Uint64

type Device struct {
	ID             int64   `json:"id"`
	StableKey      string  `json:"stable_key"`
	MAC            string  `json:"mac_address"`
	IP             string  `json:"current_ip"`
	AutoHostname   string  `json:"auto_hostname"`
	UserName       string  `json:"user_name"`
	Vendor         string  `json:"vendor"`
	AutoType       string  `json:"auto_device_type"`
	UserType       string  `json:"user_device_type"`
	Icon           string  `json:"icon"`
	Notes          string  `json:"notes"`
	FirstSeen      string  `json:"first_seen_at"`
	LastSeen       string  `json:"last_seen_at"`
	LastChecked    string  `json:"last_checked_at"`
	Status         string  `json:"status"`
	Latency        float64 `json:"ping_latency_ms"`
	ProbeMethod    string  `json:"probe_method"`
	OpenPorts      []int   `json:"open_ports"`
	TypeSource     string  `json:"identification_source"`
	TypeConfidence float64 `json:"identification_confidence"`
	Successes      int     `json:"consecutive_successes"`
	Failures       int     `json:"consecutive_failures"`
	IsNew          bool    `json:"is_new"`
	IsHidden       bool    `json:"is_hidden"`
	IsIgnored      bool    `json:"is_ignored"`
	AlwaysShow     bool    `json:"always_show"`
	Manual         bool    `json:"created_manually"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Locked         bool    `json:"locked"`
}
type Connection struct {
	ID         int64   `json:"id"`
	SourceID   int64   `json:"source_device_id"`
	TargetID   int64   `json:"target_device_id"`
	Type       string  `json:"connection_type"`
	Port       string  `json:"port_label"`
	SourceType string  `json:"source_type"`
	Confidence float64 `json:"confidence"`
	Confirmed  bool    `json:"user_confirmed"`
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "meowtopo.db")
	db, err := sql.Open("sqlite", p+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, path: p}
	if err = s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) migrate() error {
	q := `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS devices(id INTEGER PRIMARY KEY AUTOINCREMENT,stable_key TEXT NOT NULL UNIQUE,mac_address TEXT DEFAULT '',current_ip TEXT DEFAULT '',auto_hostname TEXT DEFAULT '',user_name TEXT DEFAULT '',vendor TEXT DEFAULT '',auto_device_type TEXT DEFAULT 'unknown',user_device_type TEXT DEFAULT '',icon TEXT DEFAULT '',notes TEXT DEFAULT '',first_seen_at TEXT NOT NULL,last_seen_at TEXT DEFAULT '',last_checked_at TEXT DEFAULT '',status TEXT NOT NULL DEFAULT 'unknown',ping_latency_ms REAL DEFAULT 0,consecutive_successes INTEGER DEFAULT 0,consecutive_failures INTEGER DEFAULT 0,is_new INTEGER DEFAULT 1,is_hidden INTEGER DEFAULT 0,is_ignored INTEGER DEFAULT 0,always_show INTEGER DEFAULT 0,created_manually INTEGER DEFAULT 0,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS device_addresses(id INTEGER PRIMARY KEY,device_id INTEGER NOT NULL,address TEXT NOT NULL,first_seen_at TEXT NOT NULL,last_seen_at TEXT NOT NULL,UNIQUE(device_id,address),FOREIGN KEY(device_id) REFERENCES devices(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS connections(id INTEGER PRIMARY KEY AUTOINCREMENT,source_device_id INTEGER NOT NULL,target_device_id INTEGER NOT NULL,connection_type TEXT NOT NULL DEFAULT 'unknown',port_label TEXT DEFAULT '',source_type TEXT NOT NULL DEFAULT 'inferred',confidence REAL DEFAULT .3,user_confirmed INTEGER DEFAULT 0,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(source_device_id,target_device_id),FOREIGN KEY(source_device_id) REFERENCES devices(id) ON DELETE CASCADE,FOREIGN KEY(target_device_id) REFERENCES devices(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS node_positions(device_id INTEGER PRIMARY KEY,x REAL NOT NULL,y REAL NOT NULL,locked INTEGER DEFAULT 0,updated_at TEXT NOT NULL,FOREIGN KEY(device_id) REFERENCES devices(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS scan_runs(id INTEGER PRIMARY KEY AUTOINCREMENT,started_at TEXT NOT NULL,finished_at TEXT DEFAULT '',status TEXT NOT NULL,cidrs TEXT NOT NULL,total_addresses INTEGER DEFAULT 0,scanned_addresses INTEGER DEFAULT 0,found_devices INTEGER DEFAULT 0,error_summary TEXT DEFAULT '');
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS status_events(id INTEGER PRIMARY KEY AUTOINCREMENT,device_id INTEGER,event_type TEXT,old_status TEXT,new_status TEXT,created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_devices_ip ON devices(current_ip); CREATE INDEX IF NOT EXISTS idx_events_created ON status_events(created_at);`
	if _, err := s.db.Exec(q); err != nil {
		return err
	}
	for name, definition := range map[string]string{
		"probe_method":              "TEXT NOT NULL DEFAULT ''",
		"open_ports":                "TEXT NOT NULL DEFAULT '[]'",
		"identification_source":     "TEXT NOT NULL DEFAULT ''",
		"identification_confidence": "REAL NOT NULL DEFAULT 0",
	} {
		if err := s.ensureDeviceColumn(name, definition); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version) VALUES(1),(2)`)
	return err
}
func (s *Store) ensureDeviceColumn(name, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(devices)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var column, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &column, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if column == name {
			found = true
		}
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE devices ADD COLUMN ` + name + ` ` + definition)
	return err
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }
func scanDevice(rows interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var n, h, ig, a, m, l int
	var openPorts string
	err := rows.Scan(&d.ID, &d.StableKey, &d.MAC, &d.IP, &d.AutoHostname, &d.UserName, &d.Vendor, &d.AutoType, &d.UserType, &d.Icon, &d.Notes, &d.FirstSeen, &d.LastSeen, &d.LastChecked, &d.Status, &d.Latency, &d.ProbeMethod, &openPorts, &d.TypeSource, &d.TypeConfidence, &d.Successes, &d.Failures, &n, &h, &ig, &a, &m, &d.CreatedAt, &d.UpdatedAt, &d.X, &d.Y, &l)
	if err == nil {
		_ = json.Unmarshal([]byte(openPorts), &d.OpenPorts)
	}
	d.IsNew = n != 0
	d.IsHidden = h != 0
	d.IsIgnored = ig != 0
	d.AlwaysShow = a != 0
	d.Manual = m != 0
	d.Locked = l != 0
	return d, err
}

const deviceSelect = `SELECT d.id,d.stable_key,d.mac_address,d.current_ip,d.auto_hostname,d.user_name,d.vendor,d.auto_device_type,d.user_device_type,d.icon,d.notes,d.first_seen_at,d.last_seen_at,d.last_checked_at,d.status,d.ping_latency_ms,d.probe_method,d.open_ports,d.identification_source,d.identification_confidence,d.consecutive_successes,d.consecutive_failures,d.is_new,d.is_hidden,d.is_ignored,d.always_show,d.created_manually,d.created_at,d.updated_at,COALESCE(p.x,0),COALESCE(p.y,0),COALESCE(p.locked,0) FROM devices d LEFT JOIN node_positions p ON p.device_id=d.id`

func (s *Store) devices() (out []Device, err error) {
	r, err := s.db.Query(deviceSelect + ` ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for r.Next() {
		d, e := scanDevice(r)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, r.Err()
}
func (s *Store) device(id int64) (Device, error) {
	return scanDevice(s.db.QueryRow(deviceSelect+` WHERE d.id=?`, id))
}

type Discovery struct {
	IP, MAC, Hostname, Type, ProbeMethod, TypeSource string
	Latency, TypeConfidence                          float64
	OpenPorts                                        []int
}

func (s *Store) upsertSeen(v Discovery) (Device, error) {
	t := now()
	key := "ip:" + v.IP
	if v.MAC != "" {
		key = "mac:" + v.MAC
	}
	openPorts := v.OpenPorts
	if openPorts == nil {
		openPorts = []int{}
	}
	portsJSON, _ := json.Marshal(openPorts)
	tx, e := s.db.Begin()
	if e != nil {
		return Device{}, e
	}
	defer tx.Rollback()
	var id int64
	var oldStatus string
	e = tx.QueryRow(`SELECT id,status FROM devices WHERE stable_key=? OR (?<>'' AND current_ip=?) ORDER BY stable_key=? DESC LIMIT 1`, key, v.IP, v.IP, key).Scan(&id, &oldStatus)
	if errors.Is(e, sql.ErrNoRows) {
		r, e := tx.Exec(`INSERT INTO devices(stable_key,mac_address,current_ip,auto_hostname,auto_device_type,first_seen_at,last_seen_at,last_checked_at,status,ping_latency_ms,probe_method,open_ports,identification_source,identification_confidence,consecutive_successes,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?)`, key, v.MAC, v.IP, v.Hostname, v.Type, t, t, t, "online", v.Latency, v.ProbeMethod, string(portsJSON), v.TypeSource, v.TypeConfidence, t, t)
		if e != nil {
			return Device{}, e
		}
		id, _ = r.LastInsertId()
	} else if e != nil {
		return Device{}, e
	} else {
		_, e = tx.Exec(`UPDATE devices SET stable_key=CASE WHEN mac_address='' AND ?<>'' THEN ? ELSE stable_key END,mac_address=CASE WHEN ?<>'' THEN ? ELSE mac_address END,current_ip=?,auto_hostname=CASE WHEN ?<>'' THEN ? ELSE auto_hostname END,auto_device_type=CASE WHEN ? >= identification_confidence THEN ? ELSE auto_device_type END,probe_method=?,open_ports=?,identification_source=CASE WHEN ? >= identification_confidence THEN ? ELSE identification_source END,identification_confidence=MAX(identification_confidence,?),last_seen_at=?,last_checked_at=?,status='online',ping_latency_ms=?,consecutive_successes=consecutive_successes+1,consecutive_failures=0,updated_at=? WHERE id=?`, v.MAC, key, v.MAC, v.MAC, v.IP, v.Hostname, v.Hostname, v.TypeConfidence, v.Type, v.ProbeMethod, string(portsJSON), v.TypeConfidence, v.TypeSource, v.TypeConfidence, t, t, v.Latency, t, id)
		if e != nil {
			return Device{}, e
		}
		if oldStatus != "online" {
			tx.Exec(`INSERT INTO status_events(device_id,event_type,old_status,new_status,created_at)VALUES(?,'status',?,'online',?)`, id, oldStatus, t)
		}
	}
	_, _ = tx.Exec(`INSERT INTO device_addresses(device_id,address,first_seen_at,last_seen_at)VALUES(?,?,?,?) ON CONFLICT(device_id,address) DO UPDATE SET last_seen_at=excluded.last_seen_at`, id, v.IP, t, t)
	if e = tx.Commit(); e != nil {
		return Device{}, e
	}
	return s.device(id)
}
func (s *Store) markMisses(seen map[string]bool, threshold int) error {
	ds, e := s.devices()
	if e != nil {
		return e
	}
	t := now()
	for _, d := range ds {
		if d.Manual || d.IsIgnored || seen[d.IP] {
			continue
		}
		f := d.Failures + 1
		status := "suspected_offline"
		if f >= threshold {
			status = "offline"
		}
		if d.Status == "unknown" && f == 1 {
			status = "unknown"
		}
		_, e = s.db.Exec(`UPDATE devices SET consecutive_failures=?,consecutive_successes=0,status=?,last_checked_at=?,updated_at=? WHERE id=?`, f, status, t, t, d.ID)
		if e != nil {
			return e
		}
		if status != d.Status {
			_, _ = s.db.Exec(`INSERT INTO status_events(device_id,event_type,old_status,new_status,created_at)VALUES(?,'status',?,?,?)`, d.ID, d.Status, status, t)
		}
	}
	_ = s.trimStatusEvents(5000)
	return nil
}
func (s *Store) trimStatusEvents(limit int) error {
	_, err := s.db.Exec(`DELETE FROM status_events WHERE id NOT IN (SELECT id FROM status_events ORDER BY id DESC LIMIT ?)`, limit)
	return err
}
func (s *Store) connections() (out []Connection, err error) {
	r, e := s.db.Query(`SELECT id,source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed FROM connections`)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	for r.Next() {
		var c Connection
		var b int
		if e = r.Scan(&c.ID, &c.SourceID, &c.TargetID, &c.Type, &c.Port, &c.SourceType, &c.Confidence, &b); e != nil {
			return nil, e
		}
		c.Confirmed = b != 0
		out = append(out, c)
	}
	return out, r.Err()
}
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
	case "initialized", "scan_interface", "scan_cidrs", "gateway_ip", "scan_interval", "scan_concurrency", "ping_timeout", "tcp_timeout", "offline_threshold", "enable_port_scan", "theme", "label_mode", "hide_offline_days":
		return true
	}
	return false
}
func (s *Store) createManual(name, typ, notes string) (Device, error) {
	t := now()
	r, e := s.db.Exec(`INSERT INTO devices(stable_key,user_name,user_device_type,notes,first_seen_at,status,is_new,created_manually,created_at,updated_at)VALUES(?,?,?,?,?,'unknown',0,1,?,?)`, fmt.Sprintf("manual:%d-%d", time.Now().UnixNano(), manualSequence.Add(1)), name, typ, notes, t, t, t)
	if e != nil {
		return Device{}, e
	}
	id, _ := r.LastInsertId()
	return s.device(id)
}
func (s *Store) ensureCore(gateway string) error {
	t := now()
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`INSERT INTO devices(stable_key,user_name,user_device_type,first_seen_at,status,is_new,created_manually,created_at,updated_at)VALUES('virtual:internet','Internet','internet',?,'unknown',0,1,?,?) ON CONFLICT(stable_key) DO NOTHING`, t, t, t)
	if e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT INTO devices(stable_key,current_ip,user_name,user_device_type,first_seen_at,status,is_new,created_at,updated_at)VALUES(?,?,'主网关','gateway',?,'unknown',0,?,?) ON CONFLICT(stable_key) DO UPDATE SET current_ip=excluded.current_ip`, "ip:"+gateway, gateway, t, t, t)
	if e != nil {
		return e
	}
	var internet, gatewayID int64
	if e = tx.QueryRow(`SELECT id FROM devices WHERE stable_key='virtual:internet'`).Scan(&internet); e != nil {
		return e
	}
	if e = tx.QueryRow(`SELECT id FROM devices WHERE stable_key=?`, "ip:"+gateway).Scan(&gatewayID); e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,source_type,confidence,user_confirmed,created_at,updated_at)VALUES(?,?,'virtual','manual',1,1,?,?) ON CONFLICT(source_device_id,target_device_id) DO NOTHING`, internet, gatewayID, t, t)
	if e != nil {
		return e
	}
	_, e = tx.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,source_type,confidence,user_confirmed,created_at,updated_at) SELECT ?,d.id,'logical','inferred',.25,0,?,? FROM devices d WHERE d.id NOT IN (?,?) AND NOT EXISTS(SELECT 1 FROM connections c WHERE c.target_device_id=d.id)`, gatewayID, t, t, internet, gatewayID)
	if e != nil {
		return e
	}
	return tx.Commit()
}
