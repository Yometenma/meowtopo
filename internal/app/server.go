package app

import (
	"database/sql"
	"embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/* web/assets/*
var webFS embed.FS

type Server struct {
	cfg             Config
	store           *Store
	scanner         *Scanner
	events          *EventHub
	version         string
	intervalUpdates chan time.Duration
	notifier        *Notifier
	vendors         *macVendorDatabase
	snmp            *SNMPDiscovery
	backupMu        sync.Mutex
}

func Run(version string) error {
	c := loadConfig()
	if err := os.MkdirAll(c.DataDir, 0750); err != nil {
		return err
	}
	st, err := openStore(c.DataDir)
	if err != nil {
		return err
	}
	defer st.db.Close()
	settings, _ := st.settings()
	c, err = applyStoredSettings(c, settings)
	if err != nil {
		return fmt.Errorf("读取已保存设置: %w", err)
	}
	hub := newHub()
	srv := &Server{cfg: c, store: st, events: hub, version: version, intervalUpdates: make(chan time.Duration, 1), vendors: openMACVendorDatabase(c.DataDir)}
	srv.notifier = newNotifier(st)
	srv.snmp = newSNMPDiscovery(st)
	srv.scanner = &Scanner{store: st, cfg: c, events: hub, notifier: srv.notifier, vendors: srv.vendors}
	srv.scanner.snmp = srv.snmp
	mux := http.NewServeMux()
	srv.routes(mux)
	h := securityHeaders(logRequests(mux))
	httpSrv := &http.Server{Addr: c.HTTPAddr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 0, IdleTimeout: 60 * time.Second}
	go srv.schedule(c.ScanInterval)
	go srv.maintain()
	slog.Info("MeowTopo started", "address", c.HTTPAddr, "data", c.DataDir)
	return httpSrv.ListenAndServe()
}
func (s *Server) schedule(interval time.Duration) {
	t := time.NewTimer(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			cfg := s.scanner.config()
			if len(cfg.CIDRs) > 0 {
				_ = s.scanner.Start(strings.Join(cfg.CIDRs, ","))
			}
			t.Reset(interval)
		case interval = <-s.intervalUpdates:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			t.Reset(interval)
		}
	}
}
func (s *Server) routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]any{"status": "ok", "database": s.store.db.Ping() == nil})
	})
	m.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]string{"name": "MeowTopo", "version": s.version})
	})
	m.HandleFunc("GET /api/auth/status", s.authStatus)
	m.HandleFunc("POST /api/auth/bootstrap", s.bootstrapAdmin)
	m.HandleFunc("POST /api/auth/login", s.login)
	m.Handle("GET /api/auth/me", s.require(PermView, s.me))
	m.Handle("POST /api/auth/logout", s.require(PermView, s.logout))
	m.Handle("GET /api/users", s.require(PermManageUsers, s.users))
	m.Handle("POST /api/users", s.require(PermManageUsers, s.createAccount))
	m.Handle("PATCH /api/users/{id}", s.require(PermManageUsers, s.updateAccount))
	m.Handle("GET /api/network/interfaces", s.require(PermManageSettings, func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, detectNetwork(s.cfg.DataDir).Interfaces) }))
	m.Handle("GET /api/network/detection", s.require(PermManageSettings, func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, detectNetwork(s.cfg.DataDir)) }))
	m.Handle("GET /api/devices", s.require(PermView, s.listDevices))
	m.Handle("GET /api/devices/export", s.require(PermView, s.exportDevices))
	m.Handle("POST /api/devices", s.require(PermEditDevices, s.createDevice))
	m.Handle("POST /api/devices/batch", s.require(PermEditDevices, s.batchDevices))
	m.Handle("GET /api/devices/{id}", s.require(PermView, s.getDevice))
	m.Handle("GET /api/devices/{id}/history", s.require(PermView, s.deviceHistory))
	m.Handle("PATCH /api/devices/{id}", s.require(PermEditDevices, s.patchDevice))
	m.Handle("DELETE /api/devices/{id}", s.require(PermEditDevices, s.deleteDevice))
	m.Handle("POST /api/devices/{id}/ping", s.require(PermRunScans, s.pingDevice))
	m.Handle("POST /api/devices/{id}/hide", s.require(PermEditDevices, s.visibility(true)))
	m.Handle("POST /api/devices/{id}/unhide", s.require(PermEditDevices, s.visibility(false)))
	m.Handle("PATCH /api/devices/{id}/position", s.require(PermEditDevices, s.position))
	m.Handle("PATCH /api/devices/{id}/connection", s.require(PermEditDevices, s.connection))
	m.Handle("POST /api/devices/{id}/connections", s.require(PermEditDevices, s.addConnection))
	m.Handle("DELETE /api/devices/{id}/connections/{connection}", s.require(PermEditDevices, s.deleteConnection))
	m.Handle("GET /api/topology", s.require(PermView, s.topology))
	m.Handle("POST /api/topology/layout/reset", s.require(PermEditDevices, s.resetLayout))
	m.Handle("POST /api/scan", s.require(PermRunScans, s.startScan))
	m.Handle("GET /api/scan/status", s.require(PermView, func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, s.scanner.Status()) }))
	m.Handle("GET /api/scan/history", s.require(PermView, s.scanHistory))
	m.Handle("GET /api/scan/diagnostics", s.require(PermView, s.scanDiagnostics))
	m.Handle("GET /api/status/events", s.require(PermView, s.statusEvents))
	m.Handle("GET /api/settings", s.require(PermManageSettings, s.getSettings))
	m.Handle("PATCH /api/settings", s.require(PermManageSettings, s.patchSettings))
	m.Handle("POST /api/notifications/test", s.require(PermManageSettings, s.testNotification))
	m.Handle("GET /api/vendor-database", s.require(PermManageSettings, s.vendorDatabaseStatus))
	m.Handle("POST /api/vendor-database/update", s.require(PermManageSettings, s.updateVendorDatabase))
	m.Handle("GET /api/snmp/targets", s.require(PermManageSettings, s.listSNMPTargets))
	m.Handle("POST /api/snmp/targets", s.require(PermManageSettings, s.createSNMPTarget))
	m.Handle("PATCH /api/snmp/targets/{id}", s.require(PermManageSettings, s.updateSNMPTarget))
	m.Handle("DELETE /api/snmp/targets/{id}", s.require(PermManageSettings, s.deleteSNMPTarget))
	m.Handle("POST /api/snmp/targets/{id}/test", s.require(PermManageSettings, s.testSNMPTarget))
	m.Handle("POST /api/snmp/poll", s.require(PermManageSettings, s.pollSNMP))
	m.Handle("GET /api/backup", s.require(PermManageSettings, s.backup))
	m.Handle("GET /api/maintenance", s.require(PermManageSettings, s.maintenanceStatus))
	m.Handle("POST /api/maintenance/backup", s.require(PermManageSettings, s.createAutomaticBackup))
	m.Handle("POST /api/restore", s.require(PermManageUsers, s.restore))
	m.Handle("GET /api/events", s.require(PermView, s.sse))
	root, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(root))
	m.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", staticCacheControl(r.URL.Path))
		files.ServeHTTP(w, r)
	}))
}

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
func (s *Server) startScan(w http.ResponseWriter, r *http.Request) {
	var v struct {
		CIDR string `json:"cidr"`
	}
	_ = decode(r, &v)
	if v.CIDR == "" {
		set, _ := s.store.settings()
		v.CIDR = set["scan_cidrs"]
	}
	if v.CIDR == "" {
		fail(w, 400, "not_configured", "请先配置扫描网段")
		return
	}
	if e := s.scanner.Start(v.CIDR); e != nil {
		fail(w, 409, "scan_rejected", e.Error())
		return
	}
	jsonOut(w, 202, s.scanner.Status())
}
func (s *Server) scanHistory(w http.ResponseWriter, r *http.Request) {
	rows, e := s.store.db.Query(`SELECT id,started_at,finished_at,status,cidrs,total_addresses,scanned_addresses,found_devices,error_summary FROM scan_runs ORDER BY id DESC LIMIT 50`)
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, total, scanned, found int
		var started, finished, status, cidrs, err string
		if scanErr := rows.Scan(&id, &started, &finished, &status, &cidrs, &total, &scanned, &found, &err); scanErr != nil {
			fail(w, 500, "database_error", scanErr.Error())
			return
		}
		out = append(out, map[string]any{"id": id, "started_at": started, "finished_at": finished, "status": status, "cidrs": cidrs, "total": total, "scanned": scanned, "found": found, "error": err})
	}
	if err := rows.Err(); err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	jsonOut(w, 200, out)
}

func (s *Server) scanDiagnostics(w http.ResponseWriter, r *http.Request) {
	detection := detectNetwork(s.cfg.DataDir)
	settings, _ := s.store.settings()
	latest := struct {
		FinishedAt string `json:"finished_at"`
		Status     string `json:"status"`
		CIDRs      string `json:"cidrs"`
		Total      int    `json:"total_addresses"`
		Scanned    int    `json:"scanned_addresses"`
		Found      int    `json:"found_devices"`
		Error      string `json:"error_summary"`
	}{}
	_ = s.store.db.QueryRow(`SELECT finished_at,status,cidrs,total_addresses,scanned_addresses,found_devices,error_summary FROM scan_runs ORDER BY id DESC LIMIT 1`).Scan(&latest.FinishedAt, &latest.Status, &latest.CIDRs, &latest.Total, &latest.Scanned, &latest.Found, &latest.Error)
	warnings := append([]string{}, detection.Warnings...)
	if latest.Total > 0 && latest.Found <= 2 {
		warnings = append(warnings, "发现的设备很少，请确认服务器与家庭设备处在同一局域网，并检查访客网络、VLAN 或 Docker 网络隔离。")
	}
	if latest.Total > 0 && latest.Scanned < latest.Total {
		warnings = append(warnings, "最近一次扫描没有检查完全部地址。")
	}
	if strings.TrimSpace(settings["scan_interface"]) == "" {
		warnings = append(warnings, "当前自动选择网卡；多网卡或 VPN 环境建议明确选择连接家庭网络的网卡。")
	}
	jsonOut(w, 200, map[string]any{"latest": latest, "interface": settings["scan_interface"], "configured_cidrs": settings["scan_cidrs"], "docker_likely": detection.DockerLikely, "raw_probe_available": detection.RawProbeAvailable, "warnings": warnings})
}
func (s *Server) statusEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := s.store.db.Query(`SELECT e.id,e.device_id,e.event_type,e.old_status,e.new_status,e.created_at,COALESCE(NULLIF(d.user_name,''),NULLIF(d.auto_hostname,''),NULLIF(d.current_ip,''),'已删除设备') FROM status_events e LEFT JOIN devices d ON d.id=e.device_id ORDER BY e.id DESC LIMIT ?`, limit)
	if err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, deviceID int64
		var eventType, oldStatus, newStatus, createdAt, deviceName string
		if err = rows.Scan(&id, &deviceID, &eventType, &oldStatus, &newStatus, &createdAt, &deviceName); err != nil {
			fail(w, 500, "database_error", err.Error())
			return
		}
		out = append(out, map[string]any{"id": id, "device_id": deviceID, "device_name": deviceName, "event_type": eventType, "old_status": oldStatus, "new_status": newStatus, "created_at": createdAt})
	}
	if err = rows.Err(); err != nil {
		fail(w, 500, "database_error", err.Error())
		return
	}
	jsonOut(w, 200, out)
}
