package app

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	d, e := s.store.devices()
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	jsonOut(w, 200, d)
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r)
	if e != nil {
		fail(w, 400, "invalid_id", "设备编号无效")
		return
	}
	d, e := s.store.device(id)
	if e != nil {
		fail(w, 404, "not_found", "设备不存在")
		return
	}
	jsonOut(w, 200, d)
}
func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var v struct{ Name, Type, Notes string }
	if e := decode(r, &v); e != nil || strings.TrimSpace(v.Name) == "" {
		fail(w, 400, "invalid_request", "名称不能为空")
		return
	}
	d, e := s.store.createManual(strings.TrimSpace(v.Name), safeType(v.Type), strings.TrimSpace(v.Notes))
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	s.events.Emit("device_created", d)
	jsonOut(w, 201, d)
}
func (s *Server) batchDevices(w http.ResponseWriter, r *http.Request) {
	var v struct {
		IDs            []int64 `json:"ids"`
		Action         string  `json:"action"`
		ParentID       int64   `json:"parent_id"`
		ConnectionType string  `json:"connection_type"`
	}
	if err := decode(r, &v); err != nil || len(v.IDs) == 0 || len(v.IDs) > 500 {
		fail(w, 400, "invalid_request", "请选择 1 到 500 台设备")
		return
	}
	seen := make(map[int64]bool, len(v.IDs))
	ids := make([]int64, 0, len(v.IDs))
	for _, id := range v.IDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		fail(w, 400, "invalid_request", "设备编号无效")
		return
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	tx, err := s.store.db.Begin()
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	defer tx.Rollback()
	var result sql.Result
	switch v.Action {
	case "hide":
		result, err = tx.Exec(`UPDATE devices SET is_hidden=1,updated_at=? WHERE id IN (`+placeholders+`)`, append([]any{now()}, args...)...)
	case "unhide":
		result, err = tx.Exec(`UPDATE devices SET is_hidden=0,updated_at=? WHERE id IN (`+placeholders+`)`, append([]any{now()}, args...)...)
	case "clear_new":
		result, err = tx.Exec(`UPDATE devices SET is_new=0,updated_at=? WHERE id IN (`+placeholders+`)`, append([]any{now()}, args...)...)
	case "set_parent":
		if v.ParentID <= 0 || seen[v.ParentID] {
			fail(w, 400, "invalid_parent", "父设备不能是所选设备本身")
			return
		}
		if err = tx.QueryRow(`SELECT id FROM devices WHERE id=?`, v.ParentID).Scan(&v.ParentID); err != nil {
			fail(w, 400, "invalid_parent", "父设备不存在")
			return
		}
		switch v.ConnectionType {
		case "ethernet", "wifi", "unknown", "virtual", "logical":
		default:
			v.ConnectionType = "unknown"
		}
		if _, err = tx.Exec(`DELETE FROM connections WHERE user_confirmed=0 AND target_device_id IN (`+placeholders+`)`, args...); err == nil {
			for _, id := range ids {
				var cycle bool
				cycle, err = connectionWouldCycle(tx, v.ParentID, id)
				if err == nil && cycle {
					err = fmt.Errorf("connection cycle for device %d", id)
				}
				if err == nil {
					_, err = tx.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,source_type,confidence,user_confirmed,created_at,updated_at)VALUES(?,?,?,'manual',1,1,?,?) ON CONFLICT(source_device_id,target_device_id) DO UPDATE SET connection_type=excluded.connection_type,source_type='manual',confidence=1,user_confirmed=1,updated_at=excluded.updated_at`, v.ParentID, id, v.ConnectionType, now(), now())
				}
				if err != nil {
					break
				}
			}
		}
		result = nil
	default:
		fail(w, 400, "invalid_action", "不支持的批量操作")
		return
	}
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	updated := int64(len(ids))
	if result != nil {
		updated, _ = result.RowsAffected()
	}
	s.events.Emit("devices_updated", map[string]any{"ids": ids, "action": v.Action})
	jsonOut(w, 200, map[string]any{"updated": updated})
}
func (s *Server) patchDevice(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r)
	if e != nil {
		fail(w, 400, "invalid_id", "设备编号无效")
		return
	}
	var v struct {
		UserName     *string `json:"user_name"`
		UserType     *string `json:"user_device_type"`
		Icon         *string `json:"icon"`
		Notes        *string `json:"notes"`
		IsNew        *bool   `json:"is_new"`
		IsIgnored    *bool   `json:"is_ignored"`
		AlwaysShow   *bool   `json:"always_show"`
		Important    *bool   `json:"is_important"`
		PresenceMode *string `json:"presence_mode"`
	}
	if e = decode(r, &v); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	var previous Device
	if v.UserType != nil {
		previous, e = s.store.device(id)
		if e != nil {
			fail(w, 404, "not_found", "设备不存在")
			return
		}
	}
	sets := []string{"updated_at=?"}
	args := []any{now()}
	if v.UserName != nil {
		sets = append(sets, "user_name=?")
		args = append(args, strings.TrimSpace(*v.UserName))
	}
	if v.UserType != nil {
		sets = append(sets, "user_device_type=?")
		args = append(args, safeType(*v.UserType))
	}
	if v.Icon != nil {
		sets = append(sets, "icon=?")
		args = append(args, safeType(*v.Icon))
	}
	if v.Notes != nil {
		sets = append(sets, "notes=?")
		args = append(args, strings.TrimSpace(*v.Notes))
	}
	if v.IsNew != nil {
		sets = append(sets, "is_new=?")
		args = append(args, *v.IsNew)
	}
	if v.IsIgnored != nil {
		sets = append(sets, "is_ignored=?")
		args = append(args, *v.IsIgnored)
	}
	if v.AlwaysShow != nil {
		sets = append(sets, "always_show=?")
		args = append(args, *v.AlwaysShow)
	}
	if v.Important != nil {
		sets = append(sets, "is_important=?")
		args = append(args, *v.Important)
	}
	if v.PresenceMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*v.PresenceMode))
		if mode != "normal" && mode != "occasional" {
			fail(w, 400, "invalid_presence_mode", "设备在线方式无效")
			return
		}
		sets = append(sets, "presence_mode=?")
		args = append(args, mode)
	}
	args = append(args, id)
	if _, e = s.store.db.Exec(`UPDATE devices SET `+strings.Join(sets, ",")+` WHERE id=?`, args...); e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	d, e := s.store.device(id)
	if e != nil {
		fail(w, 404, "not_found", "设备不存在")
		return
	}
	if v.UserType != nil && d.UserType != previous.UserType {
		if e = s.store.recordIdentificationCorrection(id, d.AutoType, d.UserType, d.Vendor, d.TypeEvidence); e != nil {
			fail(w, 500, "database_error", e.Error())
			return
		}
	}
	s.events.Emit("device_updated", d)
	jsonOut(w, 200, d)
}

func (s *Server) exportDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.store.devices()
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "json" {
		w.Header().Set("Content-Disposition", `attachment; filename="meowtopo-devices.json"`)
		jsonOut(w, 200, devices)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="meowtopo-devices.csv"`)
	_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"名称", "IP", "MAC", "类型", "状态", "在线方式", "主机名", "首次发现", "最后在线", "备注"})
	for _, d := range devices {
		name := d.UserName
		if name == "" {
			name = d.AutoHostname
		}
		if name == "" {
			name = d.IP
		}
		typ := d.UserType
		if typ == "" {
			typ = d.AutoType
		}
		mode := d.PresenceMode
		if d.Important {
			mode = "always_online"
		}
		_ = writer.Write([]string{name, d.IP, d.MAC, typ, d.Status, mode, d.AutoHostname, d.FirstSeen, d.LastSeen, d.Notes})
	}
	writer.Flush()
}
func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, 400, "invalid_id", "设备编号无效")
		return
	}
	d, err := s.store.device(id)
	if err != nil {
		fail(w, 404, "not_found", "设备不存在")
		return
	}
	if !d.Manual {
		fail(w, 409, "automatic_device", "自动发现的设备不能删除，可以隐藏或忽略")
		return
	}
	result, err := s.store.db.Exec(`DELETE FROM devices WHERE id=? AND created_manually=1`, id)
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	deleted, _ := result.RowsAffected()
	if deleted == 0 {
		fail(w, 404, "not_found", "设备不存在")
		return
	}
	s.events.Emit("device_deleted", map[string]int64{"device_id": id})
	jsonOut(w, 200, map[string]bool{"deleted": true})
}
func safeType(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	allowed := map[string]bool{"unknown": true, "internet": true, "modem": true, "gateway": true, "router": true, "ap": true, "switch": true, "nas": true, "linux": true, "windows": true, "macos": true, "phone": true, "tablet": true, "tv": true, "camera": true, "iot": true, "game": true, "printer": true, "room": true}
	if allowed[v] {
		return v
	}
	return "unknown"
}
func (s *Server) visibility(hidden bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, e := idParam(r)
		if e != nil {
			fail(w, 400, "invalid_id", "设备编号无效")
			return
		}
		res, e := s.store.db.Exec(`UPDATE devices SET is_hidden=?,updated_at=? WHERE id=?`, hidden, now(), id)
		if e != nil {
			fail(w, 500, "database_error", e.Error())
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			fail(w, 404, "not_found", "设备不存在")
			return
		}
		jsonOut(w, 200, map[string]bool{"hidden": hidden})
	}
}
func (s *Server) pingDevice(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r)
	if e != nil {
		fail(w, 400, "invalid_id", "设备编号无效")
		return
	}
	d, e := s.store.device(id)
	if e != nil || net.ParseIP(d.IP) == nil {
		fail(w, 400, "no_address", "设备没有有效 IP")
		return
	}
	ctx := r.Context()
	result := s.scanner.probe(ctx, d.IP)
	status := "offline"
	if result.Alive {
		status = "online"
	}
	_, _ = s.store.db.Exec(`INSERT INTO device_samples(device_id,checked_at,status,latency_ms,probe_method) VALUES(?,?,?,?,?)`, id, now(), status, result.Latency, result.Method)
	jsonOut(w, 200, map[string]any{"reachable": result.Alive, "latency_ms": result.Latency, "method": result.Method, "open_ports": result.OpenPorts})
}

func (s *Server) deviceHistory(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, 400, "invalid_id", "设备编号无效")
		return
	}
	hours := 24
	if raw := r.URL.Query().Get("hours"); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil && n >= 1 && n <= 720 {
			hours = n
		}
	}
	rows, err := s.store.db.Query(`SELECT checked_at,status,latency_ms,probe_method FROM device_samples WHERE device_id=? AND checked_at>=? ORDER BY checked_at`, id, time.Now().UTC().Add(-time.Duration(hours)*time.Hour).Format(time.RFC3339))
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	defer rows.Close()
	points := []map[string]any{}
	online, total, latencyCount := 0, 0, 0
	latencyTotal := 0.0
	for rows.Next() {
		var checkedAt, status, method string
		var latency float64
		if err = rows.Scan(&checkedAt, &status, &latency, &method); err != nil {
			fail(w, 500, "database_error", err.Error())
			return
		}
		total++
		if status == "online" {
			online++
		}
		if latency > 0 {
			latencyTotal += latency
			latencyCount++
		}
		points = append(points, map[string]any{"checked_at": checkedAt, "status": status, "latency_ms": latency, "probe_method": method})
	}
	uptime, average := 0.0, 0.0
	if total > 0 {
		uptime = float64(online) * 100 / float64(total)
	}
	if latencyCount > 0 {
		average = latencyTotal / float64(latencyCount)
	}
	jsonOut(w, 200, map[string]any{"hours": hours, "samples": total, "uptime_percent": uptime, "average_latency_ms": average, "points": points})
}
