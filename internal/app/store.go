package app

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
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
	ID             int64    `json:"id"`
	StableKey      string   `json:"stable_key"`
	MAC            string   `json:"mac_address"`
	IP             string   `json:"current_ip"`
	AutoHostname   string   `json:"auto_hostname"`
	UserName       string   `json:"user_name"`
	Vendor         string   `json:"vendor"`
	AutoType       string   `json:"auto_device_type"`
	UserType       string   `json:"user_device_type"`
	Icon           string   `json:"icon"`
	Notes          string   `json:"notes"`
	FirstSeen      string   `json:"first_seen_at"`
	LastSeen       string   `json:"last_seen_at"`
	LastChecked    string   `json:"last_checked_at"`
	Status         string   `json:"status"`
	Latency        float64  `json:"ping_latency_ms"`
	ProbeMethod    string   `json:"probe_method"`
	OpenPorts      []int    `json:"open_ports"`
	TypeSource     string   `json:"identification_source"`
	TypeConfidence float64  `json:"identification_confidence"`
	TypeEvidence   []string `json:"identification_evidence"`
	Successes      int      `json:"consecutive_successes"`
	Failures       int      `json:"consecutive_failures"`
	IsNew          bool     `json:"is_new"`
	IsHidden       bool     `json:"is_hidden"`
	IsIgnored      bool     `json:"is_ignored"`
	AlwaysShow     bool     `json:"always_show"`
	Important      bool     `json:"is_important"`
	PresenceMode   string   `json:"presence_mode"`
	StatusChanges  int      `json:"status_changes_hour"`
	Flapping       bool     `json:"is_flapping"`
	Manual         bool     `json:"created_manually"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	X              float64  `json:"x"`
	Y              float64  `json:"y"`
	Locked         bool     `json:"locked"`
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
CREATE TABLE IF NOT EXISTS identification_corrections(id INTEGER PRIMARY KEY AUTOINCREMENT,device_id INTEGER NOT NULL,automatic_type TEXT NOT NULL,corrected_type TEXT NOT NULL,vendor TEXT NOT NULL DEFAULT '',evidence TEXT NOT NULL DEFAULT '[]',created_at TEXT NOT NULL,FOREIGN KEY(device_id) REFERENCES devices(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY AUTOINCREMENT,username TEXT NOT NULL UNIQUE COLLATE NOCASE,display_name TEXT NOT NULL,password_hash TEXT NOT NULL,permissions INTEGER NOT NULL DEFAULT 1,is_admin INTEGER NOT NULL DEFAULT 0,is_active INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,last_login_at TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS sessions(token_hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL,csrf_token TEXT NOT NULL,expires_at TEXT NOT NULL,created_at TEXT NOT NULL,last_seen_at TEXT NOT NULL,FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS snmp_targets(id INTEGER PRIMARY KEY AUTOINCREMENT,address TEXT NOT NULL UNIQUE,port INTEGER NOT NULL DEFAULT 161,version TEXT NOT NULL DEFAULT '2c',community TEXT NOT NULL DEFAULT '',security_level TEXT NOT NULL DEFAULT 'noAuthNoPriv',username TEXT NOT NULL DEFAULT '',auth_protocol TEXT NOT NULL DEFAULT 'SHA',auth_password TEXT NOT NULL DEFAULT '',privacy_protocol TEXT NOT NULL DEFAULT 'AES',privacy_password TEXT NOT NULL DEFAULT '',enabled INTEGER NOT NULL DEFAULT 1,last_status TEXT NOT NULL DEFAULT '',last_error TEXT NOT NULL DEFAULT '',last_polled_at TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS login_attempts(ip TEXT PRIMARY KEY,failures INTEGER NOT NULL DEFAULT 0,blocked_until TEXT NOT NULL DEFAULT '');
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
		"identification_evidence":   "TEXT NOT NULL DEFAULT '[]'",
		"is_important":              "INTEGER NOT NULL DEFAULT 0",
		"presence_mode":             "TEXT NOT NULL DEFAULT 'normal'",
	} {
		if err := s.ensureDeviceColumn(name, definition); err != nil {
			return err
		}
	}
	if err := s.ensureTableColumn("identification_corrections", "vendor", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version) VALUES(1),(2),(3),(4),(5),(6),(7),(8)`)
	return err
}
func (s *Store) ensureDeviceColumn(name, definition string) error {
	return s.ensureTableColumn("devices", name, definition)
}
func (s *Store) ensureTableColumn(table, name, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
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
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + name + ` ` + definition)
	return err
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }
func scanDevice(rows interface{ Scan(...any) error }) (Device, error) {
	var d Device
	var n, h, ig, a, important, m, l, flapping int
	var openPorts, typeEvidence string
	err := rows.Scan(&d.ID, &d.StableKey, &d.MAC, &d.IP, &d.AutoHostname, &d.UserName, &d.Vendor, &d.AutoType, &d.UserType, &d.Icon, &d.Notes, &d.FirstSeen, &d.LastSeen, &d.LastChecked, &d.Status, &d.Latency, &d.ProbeMethod, &openPorts, &d.TypeSource, &d.TypeConfidence, &typeEvidence, &d.Successes, &d.Failures, &n, &h, &ig, &a, &important, &d.PresenceMode, &m, &d.CreatedAt, &d.UpdatedAt, &d.X, &d.Y, &l, &d.StatusChanges, &flapping)
	if err == nil {
		_ = json.Unmarshal([]byte(openPorts), &d.OpenPorts)
		_ = json.Unmarshal([]byte(typeEvidence), &d.TypeEvidence)
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

const deviceSelect = `SELECT d.id,d.stable_key,d.mac_address,d.current_ip,d.auto_hostname,d.user_name,d.vendor,d.auto_device_type,d.user_device_type,d.icon,d.notes,d.first_seen_at,d.last_seen_at,d.last_checked_at,d.status,d.ping_latency_ms,d.probe_method,d.open_ports,d.identification_source,d.identification_confidence,d.identification_evidence,d.consecutive_successes,d.consecutive_failures,d.is_new,d.is_hidden,d.is_ignored,d.always_show,d.is_important,d.presence_mode,d.created_manually,d.created_at,d.updated_at,COALESCE(p.x,0),COALESCE(p.y,0),COALESCE(p.locked,0),(SELECT COUNT(*) FROM status_events e WHERE e.device_id=d.id AND e.event_type='status' AND strftime('%s',e.created_at)>=strftime('%s','now','-1 hour')),CASE WHEN (SELECT COUNT(*) FROM status_events e WHERE e.device_id=d.id AND e.event_type='status' AND strftime('%s',e.created_at)>=strftime('%s','now','-1 hour'))>=3 THEN 1 ELSE 0 END FROM devices d LEFT JOIN node_positions p ON p.device_id=d.id`
