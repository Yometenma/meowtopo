package app

import (
	"net/http"
	"strconv"
	"strings"
)

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
