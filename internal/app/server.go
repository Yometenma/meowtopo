package app

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed web/* web/assets/*
var webFS embed.FS

type Server struct {
	cfg     Config
	store   *Store
	scanner *Scanner
	events  *EventHub
	version string
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
	hub := newHub()
	srv := &Server{cfg: c, store: st, events: hub, version: version}
	srv.scanner = &Scanner{store: st, cfg: c, events: hub}
	settings, _ := st.settings()
	if v := settings["scan_cidrs"]; v != "" {
		c.CIDRs = strings.Split(v, ",")
		srv.scanner.cfg.CIDRs = c.CIDRs
	}
	if v := settings["offline_threshold"]; v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			srv.scanner.cfg.OfflineThreshold = n
		}
	}
	mux := http.NewServeMux()
	srv.routes(mux)
	h := securityHeaders(logRequests(mux))
	httpSrv := &http.Server{Addr: c.HTTPAddr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 0, IdleTimeout: 60 * time.Second}
	if c.ScanInterval > 0 {
		go srv.schedule(c.ScanInterval)
	}
	slog.Info("MoeTopo started", "address", c.HTTPAddr, "data", c.DataDir)
	return httpSrv.ListenAndServe()
}
func (s *Server) schedule(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		if len(s.scanner.cfg.CIDRs) > 0 {
			_ = s.scanner.Start(s.scanner.cfg.CIDRs[0])
		}
	}
}
func (s *Server) routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]any{"status": "ok", "database": s.store.db.Ping() == nil})
	})
	m.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]string{"name": "MoeTopo", "version": s.version})
	})
	m.HandleFunc("GET /api/network/interfaces", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, detectNetwork(s.cfg.DataDir).Interfaces) })
	m.HandleFunc("GET /api/network/detection", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, detectNetwork(s.cfg.DataDir)) })
	m.HandleFunc("GET /api/devices", s.listDevices)
	m.HandleFunc("POST /api/devices", s.createDevice)
	m.HandleFunc("GET /api/devices/{id}", s.getDevice)
	m.HandleFunc("PATCH /api/devices/{id}", s.patchDevice)
	m.HandleFunc("POST /api/devices/{id}/ping", s.pingDevice)
	m.HandleFunc("POST /api/devices/{id}/hide", s.visibility(true))
	m.HandleFunc("POST /api/devices/{id}/unhide", s.visibility(false))
	m.HandleFunc("PATCH /api/devices/{id}/position", s.position)
	m.HandleFunc("PATCH /api/devices/{id}/connection", s.connection)
	m.HandleFunc("GET /api/topology", s.topology)
	m.HandleFunc("POST /api/topology/layout/reset", s.resetLayout)
	m.HandleFunc("POST /api/scan", s.startScan)
	m.HandleFunc("GET /api/scan/status", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, s.scanner.Status()) })
	m.HandleFunc("GET /api/scan/history", s.scanHistory)
	m.HandleFunc("GET /api/settings", s.getSettings)
	m.HandleFunc("PATCH /api/settings", s.patchSettings)
	m.HandleFunc("GET /api/backup", s.backup)
	m.HandleFunc("POST /api/restore", s.restore)
	m.HandleFunc("GET /api/events", s.sse)
	root, _ := fs.Sub(webFS, "web")
	m.Handle("/", http.FileServer(http.FS(root)))
}
func idParam(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	jsonOut(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}
func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
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
func (s *Server) patchDevice(w http.ResponseWriter, r *http.Request) {
	id, e := idParam(r)
	if e != nil {
		fail(w, 400, "invalid_id", "设备编号无效")
		return
	}
	var v struct {
		UserName   *string `json:"user_name"`
		UserType   *string `json:"user_device_type"`
		Icon       *string `json:"icon"`
		Notes      *string `json:"notes"`
		IsNew      *bool   `json:"is_new"`
		IsIgnored  *bool   `json:"is_ignored"`
		AlwaysShow *bool   `json:"always_show"`
	}
	if e = decode(r, &v); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
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
	s.events.Emit("device_updated", d)
	jsonOut(w, 200, d)
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
	ok, lat := s.scanner.probe(ctx, d.IP)
	jsonOut(w, 200, map[string]any{"reachable": ok, "latency_ms": lat, "method": "tcp_connect"})
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
	if e != nil || decode(r, &v) != nil || v.ParentID <= 0 || v.ParentID == id {
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
	if e == nil {
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
		v.CIDR = strings.Split(set["scan_cidrs"], ",")[0]
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
		rows.Scan(&id, &started, &finished, &status, &cidrs, &total, &scanned, &found, &err)
		out = append(out, map[string]any{"id": id, "started_at": started, "finished_at": finished, "status": status, "cidrs": cidrs, "total": total, "scanned": scanned, "found": found, "error": err})
	}
	jsonOut(w, 200, out)
}
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	v, e := s.store.settings()
	if e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	jsonOut(w, 200, v)
}
func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var v map[string]any
	if decode(r, &v) != nil {
		fail(w, 400, "invalid_request", "设置格式无效")
		return
	}
	if raw, ok := v["scan_cidrs"].(string); ok {
		for _, c := range strings.Split(raw, ",") {
			if _, _, e := validateCIDR(c); e != nil {
				fail(w, 400, "invalid_cidr", e.Error())
				return
			}
		}
	}
	if raw, ok := v["gateway_ip"].(string); ok && raw != "" {
		ip := net.ParseIP(raw)
		if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
			fail(w, 400, "invalid_gateway", "主网关必须是私有 IPv4 地址")
			return
		}
	}
	if e := s.store.saveSettings(v); e != nil {
		fail(w, 500, "database_error", e.Error())
		return
	}
	if raw, ok := v["gateway_ip"].(string); ok && raw != "" {
		_ = s.store.ensureCore(raw)
	}
	jsonOut(w, 200, map[string]bool{"saved": true})
}
func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	// Move committed WAL pages into the main file so the single-file backup is consistent.
	if _, e := s.store.db.Exec(`PRAGMA wal_checkpoint(FULL)`); e != nil {
		fail(w, 500, "backup_failed", "数据库检查点创建失败")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="moetopo-backup.zip"`)
	z := zip.NewWriter(w)
	f, _ := z.Create("moetopo.db")
	src, e := os.Open(s.store.path)
	if e == nil {
		_, _ = io.Copy(f, src)
		src.Close()
	}
	_ = z.Close()
}
func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	b, e := io.ReadAll(r.Body)
	if e != nil {
		fail(w, 400, "invalid_backup", "备份读取失败")
		return
	}
	zr, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		fail(w, 400, "invalid_backup", "备份不是有效 ZIP")
		return
	}
	var db []byte
	for _, f := range zr.File {
		if f.Name == "moetopo.db" && f.UncompressedSize64 <= 64<<20 {
			rc, _ := f.Open()
			db, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if len(db) < 16 || string(db[:15]) != "SQLite format 3" {
		fail(w, 400, "invalid_backup", "备份不含有效数据库")
		return
	}
	tmp := s.store.path + ".restore"
	if e = os.WriteFile(tmp, db, 0600); e != nil {
		fail(w, 500, "restore_failed", e.Error())
		return
	}
	test, e := sqlOpenCheck(tmp)
	if e != nil {
		os.Remove(tmp)
		fail(w, 400, "invalid_backup", e.Error())
		return
	}
	test.Close()
	s.store.db.Close()
	previous := s.store.path + ".pre-restore-" + time.Now().UTC().Format("20060102T150405Z")
	if e = os.Rename(s.store.path, previous); e != nil {
		fail(w, 500, "restore_failed", e.Error())
		return
	}
	if e = os.Rename(tmp, s.store.path); e != nil {
		_ = os.Rename(previous, s.store.path)
		fail(w, 500, "restore_failed", e.Error())
		return
	}
	s.store, e = openStore(filepath.Dir(s.store.path))
	if e != nil {
		fail(w, 500, "restore_failed", e.Error())
		return
	}
	s.scanner.store = s.store
	jsonOut(w, 200, map[string]string{"status": "restored", "message": "恢复完成"})
}
func sqlOpenCheck(p string) (io.Closer, error) {
	db, e := sql.Open("sqlite", p)
	if e != nil {
		return nil, e
	}
	var ok string
	e = db.QueryRow(`PRAGMA integrity_check`).Scan(&ok)
	if e != nil || ok != "ok" {
		db.Close()
		return nil, fmt.Errorf("数据库完整性检查失败")
	}
	return db, nil
}
func (s *Server) sse(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "unsupported", "不支持事件流")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	c := s.events.subscribe()
	defer s.events.unsubscribe(c)
	fmt.Fprint(w, "retry: 3000\n\n")
	f.Flush()
	for {
		select {
		case b := <-c:
			fmt.Fprintf(w, "data: %s\n\n", b)
			f.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
}
