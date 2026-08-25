package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) position(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r)
	var v struct {
		X, Y   float64
		Locked bool
	}
	if e != nil || decode(r, &v) != nil {
		fail(w, 400, "invalid_request", "坐标无效")
		return
	}
	_, e = s.store.db.Exec(`INSERT INTO node_positions(device_id,x,y,locked,updated_at)VALUES(?,?,?,?,?) ON CONFLICT(device_id) DO UPDATE SET x=excluded.x,y=excluded.y,locked=excluded.locked,updated_at=excluded.updated_at`, id, v.X, v.Y, v.Locked, now())
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	jsonOut(w, 200, map[string]bool{"saved": true})
}
func (s *Server) connection(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r)
	var v struct {
		ParentID       int64  `json:"parent_id"`
		ConnectionType string `json:"connection_type"`
		PortLabel      string `json:"port_label"`
	}
	if e != nil || decode(r, &v) != nil || v.ParentID < 0 || v.ParentID == id {
		fail(w, 400, "invalid_request", "父设备或连接信息无效")
		return
	}
	switch v.ConnectionType {
	case "ethernet", "wifi", "unknown", "virtual", "logical":
	default:
		v.ConnectionType = "unknown"
	}
	tx, e := s.store.db.Begin()
	if e == nil {
		_, e = tx.Exec(`DELETE FROM connections WHERE target_device_id=?`, id)
	}
	if e == nil && v.ParentID > 0 {
		_, e = tx.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed,created_at,updated_at)VALUES(?,?,?,?, 'manual',1,1,?,?)`, v.ParentID, id, v.ConnectionType, strings.TrimSpace(v.PortLabel), now(), now())
	}
	if e == nil {
		e = tx.Commit()
	} else {
		tx.Rollback()
	}
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	s.events.Emit("topology_changed", map[string]int64{"device_id": id})
	jsonOut(w, 200, map[string]bool{"saved": true})
}

func normalizeConnectionType(value string) string {
	switch value {
	case "ethernet", "wifi", "unknown", "virtual", "logical":
		return value
	default:
		return "unknown"
	}
}

func connectionWouldCycle(tx *sql.Tx, parentID, childID int64) (bool, error) {
	var found int
	err := tx.QueryRow(`WITH RECURSIVE descendants(id) AS (
		SELECT target_device_id FROM connections WHERE source_device_id=?
		UNION
		SELECT c.target_device_id FROM connections c JOIN descendants d ON c.source_device_id=d.id
	) SELECT 1 FROM descendants WHERE id=? LIMIT 1`, childID, parentID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// addConnection adds or updates one parent relation without removing the
// device's other user-confirmed relations. The first manual relation replaces
// only low-confidence inferred defaults, which avoids keeping a misleading
// gateway edge next to the relation the user just confirmed.
func (s *Server) addConnection(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	var v struct {
		ParentID       int64  `json:"parent_id"`
		ConnectionType string `json:"connection_type"`
		PortLabel      string `json:"port_label"`
	}
	if err != nil || decode(r, &v) != nil || v.ParentID <= 0 || v.ParentID == id {
		fail(w, 400, "invalid_request", "请选择有效的上级设备")
		return
	}
	v.ConnectionType = normalizeConnectionType(v.ConnectionType)
	tx, err := s.store.db.Begin()
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM devices WHERE id IN (?,?)`, id, v.ParentID).Scan(&count); err != nil || count != 2 {
		fail(w, 400, "invalid_parent", "设备或上级设备不存在")
		return
	}
	cycle, err := connectionWouldCycle(tx, v.ParentID, id)
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	if cycle {
		fail(w, 400, "connection_cycle", "这条连接会形成循环，请选择其他上级设备")
		return
	}
	t := now()
	if _, err = tx.Exec(`DELETE FROM connections WHERE target_device_id=? AND user_confirmed=0`, id); err == nil {
		_, err = tx.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed,created_at,updated_at)
			VALUES(?,?,?,?, 'manual',1,1,?,?)
			ON CONFLICT(source_device_id,target_device_id) DO UPDATE SET connection_type=excluded.connection_type,port_label=excluded.port_label,source_type='manual',confidence=1,user_confirmed=1,updated_at=excluded.updated_at`, v.ParentID, id, v.ConnectionType, strings.TrimSpace(v.PortLabel), t, t)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	s.events.Emit("topology_changed", map[string]int64{"device_id": id})
	jsonOut(w, 200, map[string]bool{"saved": true})
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	connectionID, parseErr := strconv.ParseInt(r.PathValue("connection"), 10, 64)
	if err != nil || parseErr != nil || connectionID <= 0 {
		fail(w, 400, "invalid_request", "连接编号无效")
		return
	}
	result, err := s.store.db.Exec(`DELETE FROM connections WHERE id=? AND target_device_id=?`, connectionID, id)
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	removed, _ := result.RowsAffected()
	if removed == 0 {
		fail(w, 404, "not_found", "连接不存在")
		return
	}
	s.events.Emit("topology_changed", map[string]int64{"device_id": id})
	jsonOut(w, 200, map[string]bool{"deleted": true})
}
func (s *Server) topology(w http.ResponseWriter, r *http.Request) {
	d, e := s.store.devices()
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	c, e := s.store.connections()
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	jsonOut(w, 200, map[string]any{"devices": d, "connections": c})
}
func (s *Server) resetLayout(w http.ResponseWriter, r *http.Request) {
	_, e := s.store.db.Exec(`DELETE FROM node_positions`)
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	jsonOut(w, 200, map[string]bool{"reset": true})
}
