package app

import (
	"database/sql"
	"encoding/json"
	"errors"
)

type Discovery struct {
	IP, MAC, Hostname, Vendor, Type, ProbeMethod, TypeSource string
	Latency, TypeConfidence                                  float64
	OpenPorts                                                []int
	TypeEvidence                                             []string
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
	typeEvidence := v.TypeEvidence
	if typeEvidence == nil {
		typeEvidence = []string{}
	}
	evidenceJSON, _ := json.Marshal(typeEvidence)
	tx, e := s.db.Begin()
	if e != nil {
		return Device{}, e
	}
	defer tx.Rollback()
	var id int64
	var oldStatus string
	e = tx.QueryRow(`SELECT id,status FROM devices WHERE stable_key=? OR (?<>'' AND current_ip=?) ORDER BY stable_key=? DESC LIMIT 1`, key, v.IP, v.IP, key).Scan(&id, &oldStatus)
	if errors.Is(e, sql.ErrNoRows) {
		r, e := tx.Exec(`INSERT INTO devices(stable_key,mac_address,current_ip,auto_hostname,vendor,auto_device_type,first_seen_at,last_seen_at,last_checked_at,status,ping_latency_ms,probe_method,open_ports,identification_source,identification_confidence,identification_evidence,consecutive_successes,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?)`, key, v.MAC, v.IP, v.Hostname, v.Vendor, v.Type, t, t, t, "online", v.Latency, v.ProbeMethod, string(portsJSON), v.TypeSource, v.TypeConfidence, string(evidenceJSON), t, t)
		if e != nil {
			return Device{}, e
		}
		id, _ = r.LastInsertId()
	} else if e != nil {
		return Device{}, e
	} else {
		_, e = tx.Exec(`UPDATE devices SET stable_key=CASE WHEN mac_address='' AND ?<>'' THEN ? ELSE stable_key END,mac_address=CASE WHEN ?<>'' THEN ? ELSE mac_address END,current_ip=?,auto_hostname=CASE WHEN ?<>'' THEN ? ELSE auto_hostname END,vendor=CASE WHEN ?<>'' THEN ? ELSE vendor END,auto_device_type=CASE WHEN ? >= identification_confidence THEN ? ELSE auto_device_type END,probe_method=?,open_ports=?,identification_source=CASE WHEN ? >= identification_confidence THEN ? ELSE identification_source END,identification_evidence=CASE WHEN ? >= identification_confidence THEN ? ELSE identification_evidence END,identification_confidence=MAX(identification_confidence,?),last_seen_at=?,last_checked_at=?,status='online',ping_latency_ms=?,consecutive_successes=consecutive_successes+1,consecutive_failures=0,updated_at=? WHERE id=?`, v.MAC, key, v.MAC, v.MAC, v.IP, v.Hostname, v.Hostname, v.Vendor, v.Vendor, v.TypeConfidence, v.Type, v.ProbeMethod, string(portsJSON), v.TypeConfidence, v.TypeSource, v.TypeConfidence, string(evidenceJSON), v.TypeConfidence, t, t, v.Latency, t, id)
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
