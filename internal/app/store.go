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
	Important      bool    `json:"is_important"`
	PresenceMode   string  `json:"presence_mode"`
	StatusChanges  int     `json:"status_changes_hour"`
	Flapping       bool    `json:"is_flapping"`
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
CREATE TABLE IF NOT EXISTS device_samples(id INTEGER PRIMARY KEY AUTOINCREMENT,device_id INTEGER NOT NULL,checked_at TEXT NOT NULL,status TEXT NOT NULL,latency_ms REAL NOT NULL DEFAULT 0,probe_method TEXT NOT NULL DEFAULT '',FOREIGN KEY(device_id) REFERENCES devices(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS notification_state(device_id INTEGER NOT NULL,event_type TEXT NOT NULL,last_sent_at TEXT NOT NULL,PRIMARY KEY(device_id,event_type),FOREIGN KEY(device_id) REFERENCES devices(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY AUTOINCREMENT,username TEXT NOT NULL UNIQUE COLLATE NOCASE,display_name TEXT NOT NULL,password_hash TEXT NOT NULL,permissions INTEGER NOT NULL DEFAULT 1,is_admin INTEGER NOT NULL DEFAULT 0,is_active INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,last_login_at TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL,csrf_token TEXT NOT NULL,expires_at TEXT NOT NULL,created_at TEXT NOT NULL,last_seen_at TEXT NOT NULL,FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id); CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_devices_ip ON devices(current_ip); CREATE INDEX IF NOT EXISTS idx_events_created ON status_events(created_at); CREATE INDEX IF NOT EXISTS idx_samples_device_time ON device_samples(device_id,checked_at);`
	if _, err := s.db.Exec(q); err != nil {
		return err
	}
	for name, definition := range map[string]string{
		"probe_method":              "TEXT NOT NULL DEFAULT ''",
		"open_ports":                "TEXT NOT NULL DEFAULT '[]'",
		"identification_source":     "TEXT NOT NULL DEFAULT ''",
		"identification_confidence": "REAL NOT NULL DEFAULT 0",
		"is_important":              "INTEGER NOT NULL DEFAULT 0",
		"presence_mode":             "TEXT NOT NULL DEFAULT 'normal'",
	} {
		if err := s.ensureDeviceColumn(name, definition); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version) VALUES(1),(2),(3),(4)`)
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
	var n, h, ig, a, important, m, l, flapping int
	var openPorts string
	err := rows.Scan(&d.ID, &d.StableKey, &d.MAC, &d.IP, &d.AutoHostname, &d.UserName, &d.Vendor, &d.AutoType, &d.UserType, &d.Icon, &d.Notes, &d.FirstSeen, &d.LastSeen, &d.LastChecked, &d.Status, &d.Latency, &d.ProbeMethod, &openPorts, &d.TypeSource, &d.TypeConfidence, &d.Successes, &d.Failures, &n, &h, &ig, &a, &important, &d.PresenceMode, &m, &d.CreatedAt, &d.UpdatedAt, &d.X, &d.Y, &l, &d.StatusChanges, &flapping)
	if err == nil {
		_ = json.Unmarshal([]byte(openPorts), &d.OpenPorts)
	}
	d.IsNew = n != 0
	d.IsHidden = h != 0
	d.IsIgnored = ig != 0
	d.AlwaysShow = a != 0
	d.Important = important != 0
	if d.PresenceMode == "" {
		d.PresenceMode = "normal"
	}
	d.Flapping = flapping != 0
	d.Manual = m != 0
	d.Locked = l != 0
	return d, err
}

const deviceSelect = `SELECT d.id,d.stable_key,d.mac_address,d.current_ip,d.auto_hostname,d.user_name,d.vendor,d.auto_device_type,d.user_device_type,d.icon,d.notes,d.first_seen_at,d.last_seen_at,d.last_checked_at,d.status,d.ping_latency_ms,d.probe_method,d.open_ports,d.identification_source,d.identification_confidence,d.consecutive_successes,d.consecutive_failures,d.is_new,d.is_hidden,d.is_ignored,d.always_show,d.is_important,d.presence_mode,d.created_manually,d.created_at,d.updated_at,COALESCE(p.x,0),COALESCE(p.y,0),COALESCE(p.locked,0),(SELECT COUNT(*) FROM status_events e WHERE e.device_id=d.id AND e.event_type='status' AND strftime('%s',e.created_at)>=strftime('%s','now','-1 hour')),CASE WHEN (SELECT COUNT(*) FROM status_events e WHERE e.device_id=d.id AND e.event_type='status' AND strftime('%s',e.created_at)>=strftime('%s','now','-1 hour'))>=3 THEN 1 ELSE 0 END FROM devices d LEFT JOIN node_positions p ON p.device_id=d.id`

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
	IP, MAC, Hostname, Vendor, Type, ProbeMethod, TypeSource string
	Latency, TypeConfidence                                  float64
	OpenPorts                                                []int
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
		r, e := tx.Exec(`INSERT INTO devices(stable_key,mac_address,current_ip,auto_hostname,vendor,auto_device_type,first_seen_at,last_seen_at,last_checked_at,status,ping_latency_ms,probe_method,open_ports,identification_source,identification_confidence,consecutive_successes,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?)`, key, v.MAC, v.IP, v.Hostname, v.Vendor, v.Type, t, t, t, "online", v.Latency, v.ProbeMethod, string(portsJSON), v.TypeSource, v.TypeConfidence, t, t)
		if e != nil {
			return Device{}, e
		}
		id, _ = r.LastInsertId()
	} else if e != nil {
		return Device{}, e
	} else {
		_, e = tx.Exec(`UPDATE devices SET stable_key=CASE WHEN mac_address='' AND ?<>'' THEN ? ELSE stable_key END,mac_address=CASE WHEN ?<>'' THEN ? ELSE mac_address END,current_ip=?,auto_hostname=CASE WHEN ?<>'' THEN ? ELSE auto_hostname END,vendor=CASE WHEN ?<>'' THEN ? ELSE vendor END,auto_device_type=CASE WHEN ? >= identification_confidence THEN ? ELSE auto_device_type END,probe_method=?,open_ports=?,identification_source=CASE WHEN ? >= identification_confidence THEN ? ELSE identification_source END,identification_confidence=MAX(identification_confidence,?),last_seen_at=?,last_checked_at=?,status='online',ping_latency_ms=?,consecutive_successes=consecutive_successes+1,consecutive_failures=0,updated_at=? WHERE id=?`, v.MAC, key, v.MAC, v.MAC, v.IP, v.Hostname, v.Hostname, v.Vendor, v.Vendor, v.TypeConfidence, v.Type, v.ProbeMethod, string(portsJSON), v.TypeConfidence, v.TypeSource, v.TypeConfidence, t, t, v.Latency, t, id)
		if e != nil {
			return Device{}, e
		}
		if oldStatus != "online" {
			tx.Exec(`INSERT INTO status_events(device_id,event_type,old_status,new_status,created_at)VALUES(?,'status',?,'online',?)`, id, oldStatus, t)
		}
	}
	_, _ = tx.Exec(`INSERT INTO device_addresses(device_id,address,first_seen_at,last_seen_at)VALUES(?,?,?,?) ON CONFLICT(device_id,address) DO UPDATE SET last_seen_at=excluded.last_seen_at`, id, v.IP, t, t)
	_, _ = tx.Exec(`INSERT INTO device_samples(device_id,checked_at,status,latency_ms,probe_method) VALUES(?,?,'online',?,?)`, id, t, v.Latency, v.ProbeMethod)
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
		deviceThreshold := threshold
		if d.PresenceMode == "occasional" {
			deviceThreshold = threshold * 4
			if deviceThreshold < 12 {
				deviceThreshold = 12
			}
		}
		status := "suspected_offline"
		if f >= deviceThreshold {
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
		_, _ = s.db.Exec(`INSERT INTO device_samples(device_id,checked_at,status,latency_ms,probe_method) VALUES(?,?,?,0,'')`, d.ID, t, status)
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
	var internet, gatewayID int64
	if e = tx.QueryRow(`SELECT id FROM devices WHERE stable_key='virtual:internet'`).Scan(&internet); e != nil {
		return e
	}
	rows, err := tx.Query(`SELECT id FROM devices WHERE current_ip=? OR stable_key=? ORDER BY (mac_address<>'') DESC,(status='online') DESC,id`, gateway, "ip:"+gateway)
	if err != nil {
		return err
	}
	var gatewayIDs []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		gatewayIDs = append(gatewayIDs, id)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(gatewayIDs) == 0 {
		result, err := tx.Exec(`INSERT INTO devices(stable_key,current_ip,user_name,user_device_type,first_seen_at,status,is_new,created_at,updated_at)VALUES(?,?,'主网关','gateway',?,'unknown',0,?,?)`, "ip:"+gateway, gateway, t, t, t)
		if err != nil {
			return err
		}
		gatewayID, _ = result.LastInsertId()
	} else {
		gatewayID = gatewayIDs[0]
		for _, duplicateID := range gatewayIDs[1:] {
			if err = mergeDeviceRecords(tx, gatewayID, duplicateID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`UPDATE devices SET current_ip=?,user_name=CASE WHEN user_name='' THEN '主网关' ELSE user_name END,user_device_type=CASE WHEN user_device_type='' THEN 'gateway' ELSE user_device_type END,is_new=0,updated_at=? WHERE id=?`, gateway, t, gatewayID); err != nil {
			return err
		}
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

func mergeDeviceRecords(tx *sql.Tx, keepID, removeID int64) error {
	if keepID == removeID {
		return nil
	}
	if _, err := tx.Exec(`UPDATE devices SET
		user_name=CASE WHEN user_name='' THEN (SELECT user_name FROM devices WHERE id=?) ELSE user_name END,
		user_device_type=CASE WHEN user_device_type='' THEN (SELECT user_device_type FROM devices WHERE id=?) ELSE user_device_type END,
		notes=CASE WHEN notes='' THEN (SELECT notes FROM devices WHERE id=?) ELSE notes END,
		is_hidden=MAX(is_hidden,(SELECT is_hidden FROM devices WHERE id=?)),
		is_ignored=MAX(is_ignored,(SELECT is_ignored FROM devices WHERE id=?)),
		always_show=MAX(always_show,(SELECT always_show FROM devices WHERE id=?)),
		is_important=MAX(is_important,(SELECT is_important FROM devices WHERE id=?)),
		presence_mode=CASE WHEN presence_mode='normal' THEN (SELECT presence_mode FROM devices WHERE id=?) ELSE presence_mode END WHERE id=?`, removeID, removeID, removeID, removeID, removeID, removeID, removeID, removeID, keepID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO device_addresses(device_id,address,first_seen_at,last_seen_at) SELECT ?,address,first_seen_at,last_seen_at FROM device_addresses WHERE device_id=?`, keepID, removeID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO node_positions(device_id,x,y,locked,updated_at) SELECT ?,x,y,locked,updated_at FROM node_positions WHERE device_id=?`, keepID, removeID); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed,created_at,updated_at FROM connections WHERE source_device_id=? OR target_device_id=?`, removeID, removeID)
	if err != nil {
		return err
	}
	type edge struct {
		source, target                           int64
		kind, port, sourceType, created, updated string
		confidence                               float64
		confirmed                                int
	}
	var edges []edge
	for rows.Next() {
		var edge edge
		if err = rows.Scan(&edge.source, &edge.target, &edge.kind, &edge.port, &edge.sourceType, &edge.confidence, &edge.confirmed, &edge.created, &edge.updated); err != nil {
			rows.Close()
			return err
		}
		edges = append(edges, edge)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM connections WHERE source_device_id=? OR target_device_id=?`, removeID, removeID); err != nil {
		return err
	}
	for _, edge := range edges {
		if edge.source == removeID {
			edge.source = keepID
		}
		if edge.target == removeID {
			edge.target = keepID
		}
		if edge.source == edge.target {
			continue
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO connections(source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, edge.source, edge.target, edge.kind, edge.port, edge.sourceType, edge.confidence, edge.confirmed, edge.created, edge.updated); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE status_events SET device_id=? WHERE device_id=?`, keepID, removeID); err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM devices WHERE id=?`, removeID)
	return err
}
