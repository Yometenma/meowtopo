package app

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

type snmpTargetInput struct {
	Address         string `json:"address"`
	Port            uint16 `json:"port"`
	Version         string `json:"version"`
	Community       string `json:"community"`
	SecurityLevel   string `json:"security_level"`
	Username        string `json:"username"`
	AuthProtocol    string `json:"auth_protocol"`
	AuthPassword    string `json:"auth_password"`
	PrivacyProtocol string `json:"privacy_protocol"`
	PrivacyPassword string `json:"privacy_password"`
	Enabled         bool   `json:"enabled"`
}

func (input snmpTargetInput) target(id int64) snmpTargetSecret {
	return snmpTargetSecret{SNMPTarget: SNMPTarget{ID: id, Address: strings.TrimSpace(input.Address), Port: input.Port, Version: input.Version, SecurityLevel: input.SecurityLevel, Username: strings.TrimSpace(input.Username), AuthProtocol: input.AuthProtocol, PrivacyProtocol: input.PrivacyProtocol, Enabled: input.Enabled}, Community: input.Community, AuthPassword: input.AuthPassword, PrivacyPassword: input.PrivacyPassword}
}

func (s *Server) listSNMPTargets(w http.ResponseWriter, _ *http.Request) {
	targets, err := s.store.snmpTargets()
	if err != nil {
		fail(w, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"targets": targets, "status": s.snmp.Status()})
}

func (s *Server) createSNMPTarget(w http.ResponseWriter, r *http.Request) {
	var input snmpTargetInput
	if err := decode(r, &input); err != nil {
		fail(w, http.StatusBadRequest, "invalid_request", "SNMP 设备设置格式无效")
		return
	}
	target, err := s.store.saveSNMPTarget(input.target(0))
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_snmp_target", friendlySNMPError(err))
		return
	}
	jsonOut(w, http.StatusCreated, target)
}

func (s *Server) updateSNMPTarget(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_id", "SNMP 设备编号无效")
		return
	}
	var input snmpTargetInput
	if err = decode(r, &input); err != nil {
		fail(w, http.StatusBadRequest, "invalid_request", "SNMP 设备设置格式无效")
		return
	}
	target, err := s.store.saveSNMPTarget(input.target(id))
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_snmp_target", friendlySNMPError(err))
		return
	}
	jsonOut(w, http.StatusOK, target)
}

func (s *Server) deleteSNMPTarget(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_id", "SNMP 设备编号无效")
		return
	}
	if err = s.store.deleteSNMPTarget(id); errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "not_found", "SNMP 设备不存在")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "database_error", err.Error())
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) testSNMPTarget(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_id", "SNMP 设备编号无效")
		return
	}
	target, err := s.store.snmpTargetSecret(id)
	if err != nil {
		fail(w, http.StatusNotFound, "not_found", "SNMP 设备不存在")
		return
	}
	client, err := makeSNMPClient(target)
	if err == nil {
		err = client.Connect()
	}
	if err != nil {
		_ = s.store.updateSNMPTargetStatus(id, "failed", err.Error())
		fail(w, http.StatusBadGateway, "snmp_connection_failed", friendlySNMPError(err))
		return
	}
	defer client.Conn.Close()
	packet, err := client.Get([]string{oidSysName, oidSysDescr})
	if err != nil || packet == nil || len(packet.Variables) < 2 {
		_ = s.store.updateSNMPTargetStatus(id, "failed", friendlySNMPError(err))
		fail(w, http.StatusBadGateway, "snmp_connection_failed", friendlySNMPError(err))
		return
	}
	_ = s.store.updateSNMPTargetStatus(id, "ok", "")
	jsonOut(w, http.StatusOK, map[string]string{"name": pduText(packet.Variables[0]), "description": pduText(packet.Variables[1])})
}

func (s *Server) pollSNMP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.snmp.Poll(ctx); err != nil {
		fail(w, http.StatusBadGateway, "snmp_poll_failed", friendlySNMPError(err))
		return
	}
	s.events.Emit("topology_updated", s.snmp.Status())
	jsonOut(w, http.StatusOK, s.snmp.Status())
}

func friendlySNMPError(err error) string {
	if err == nil {
		return "设备没有返回完整的 SNMP 信息"
	}
	message := err.Error()
	if strings.Contains(strings.ToLower(message), "timeout") || strings.Contains(strings.ToLower(message), "deadline") {
		return "设备没有响应。请检查 IP、端口、SNMP 是否启用以及认证信息。"
	}
	return message
}
