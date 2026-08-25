package app

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
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
