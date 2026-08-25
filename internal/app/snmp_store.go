package app

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) snmpTargets() ([]SNMPTarget, error) {
	secrets, err := s.snmpTargetSecrets()
	if err != nil {
		return nil, err
	}
	out := make([]SNMPTarget, 0, len(secrets))
	for _, target := range secrets {
		out = append(out, target.SNMPTarget)
	}
	return out, nil
}

func (s *Store) snmpTargetSecrets() ([]snmpTargetSecret, error) {
	rows, err := s.db.Query(`SELECT id,address,port,version,community,security_level,username,auth_protocol,auth_password,privacy_protocol,privacy_password,enabled,last_status,last_error,last_polled_at FROM snmp_targets ORDER BY address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []snmpTargetSecret
	for rows.Next() {
		var target snmpTargetSecret
		var port, enabled int
		if err = rows.Scan(&target.ID, &target.Address, &port, &target.Version, &target.Community, &target.SecurityLevel, &target.Username, &target.AuthProtocol, &target.AuthPassword, &target.PrivacyProtocol, &target.PrivacyPassword, &enabled, &target.LastStatus, &target.LastError, &target.LastPolledAt); err != nil {
			return nil, err
		}
		target.Port, target.Enabled = uint16(port), enabled != 0
		target.CommunitySet = target.Community != ""
		target.AuthPasswordSet = target.AuthPassword != ""
		target.PrivPasswordSet = target.PrivacyPassword != ""
		out = append(out, target)
	}
	return out, rows.Err()
}

func (s *Store) saveSNMPTarget(target snmpTargetSecret) (SNMPTarget, error) {
	if target.Port == 0 {
		target.Port = 161
	}
	t := now()
	if target.ID == 0 {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM snmp_targets`).Scan(&count); err != nil {
			return SNMPTarget{}, err
		}
		if count >= 32 {
			return SNMPTarget{}, errors.New("最多可以配置 32 台受管网络设备")
		}
		if err := validateSNMPTarget(target); err != nil {
			return SNMPTarget{}, err
		}
		result, err := s.db.Exec(`INSERT INTO snmp_targets(address,port,version,community,security_level,username,auth_protocol,auth_password,privacy_protocol,privacy_password,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, target.Address, target.Port, target.Version, target.Community, target.SecurityLevel, target.Username, target.AuthProtocol, target.AuthPassword, target.PrivacyProtocol, target.PrivacyPassword, target.Enabled, t, t)
		if err != nil {
			return SNMPTarget{}, err
		}
		target.ID, _ = result.LastInsertId()
	} else {
		current, err := s.snmpTargetSecret(target.ID)
		if err != nil {
			return SNMPTarget{}, err
		}
		if target.Community == "" {
			target.Community = current.Community
		}
		if target.AuthPassword == "" {
			target.AuthPassword = current.AuthPassword
		}
		if target.PrivacyPassword == "" {
			target.PrivacyPassword = current.PrivacyPassword
		}
		if err = validateSNMPTarget(target); err != nil {
			return SNMPTarget{}, err
		}
		_, err = s.db.Exec(`UPDATE snmp_targets SET address=?,port=?,version=?,community=?,security_level=?,username=?,auth_protocol=?,auth_password=?,privacy_protocol=?,privacy_password=?,enabled=?,updated_at=? WHERE id=?`, target.Address, target.Port, target.Version, target.Community, target.SecurityLevel, target.Username, target.AuthProtocol, target.AuthPassword, target.PrivacyProtocol, target.PrivacyPassword, target.Enabled, t, target.ID)
		if err != nil {
			return SNMPTarget{}, err
		}
	}
	stored, err := s.snmpTargetSecret(target.ID)
	return stored.SNMPTarget, err
}

func (s *Store) snmpTargetSecret(id int64) (snmpTargetSecret, error) {
	var target snmpTargetSecret
	var port, enabled int
	err := s.db.QueryRow(`SELECT id,address,port,version,community,security_level,username,auth_protocol,auth_password,privacy_protocol,privacy_password,enabled,last_status,last_error,last_polled_at FROM snmp_targets WHERE id=?`, id).Scan(&target.ID, &target.Address, &port, &target.Version, &target.Community, &target.SecurityLevel, &target.Username, &target.AuthProtocol, &target.AuthPassword, &target.PrivacyProtocol, &target.PrivacyPassword, &enabled, &target.LastStatus, &target.LastError, &target.LastPolledAt)
	target.Port, target.Enabled = uint16(port), enabled != 0
	target.CommunitySet, target.AuthPasswordSet, target.PrivPasswordSet = target.Community != "", target.AuthPassword != "", target.PrivacyPassword != ""
	return target, err
}

func (s *Store) deleteSNMPTarget(id int64) error {
	result, err := s.db.Exec(`DELETE FROM snmp_targets WHERE id=?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) updateSNMPTargetStatus(id int64, status, message string) error {
	_, err := s.db.Exec(`UPDATE snmp_targets SET last_status=?,last_error=?,last_polled_at=?,updated_at=? WHERE id=?`, status, message, now(), now(), id)
	return err
}

func (s *Store) matchLLDPNeighbor(chassisID, systemName string) (Device, error) {
	chassisID = strings.ToLower(strings.TrimSpace(chassisID))
	systemName = strings.TrimSpace(strings.TrimSuffix(systemName, "."))
	if chassisID != "" {
		if device, err := scanDevice(s.db.QueryRow(deviceSelect+` WHERE lower(d.mac_address)=?`, chassisID)); err == nil {
			return device, nil
		}
	}
	if systemName != "" {
		return scanDevice(s.db.QueryRow(deviceSelect+` WHERE lower(trim(d.auto_hostname,'.'))=lower(?) OR lower(d.user_name)=lower(?) ORDER BY d.user_name<>'' DESC LIMIT 1`, systemName, systemName))
	}
	return Device{}, sql.ErrNoRows
}

func (s *Store) upsertDiscoveredConnection(sourceID, targetID int64, connectionType, port, sourceType string, confidence float64) (bool, error) {
	if sourceID == targetID {
		return false, errors.New("设备不能连接到自身")
	}
	var confirmed int
	err := s.db.QueryRow(`SELECT user_confirmed FROM connections WHERE source_device_id=? AND target_device_id=?`, sourceID, targetID).Scan(&confirmed)
	if err == nil && confirmed != 0 {
		return false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var reverse int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM connections WHERE source_device_id=? AND target_device_id=?`, targetID, sourceID).Scan(&reverse)
	if reverse > 0 {
		return false, nil
	}
	result, err := s.db.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed,created_at,updated_at) VALUES(?,?,?,?,?,?,0,?,?) ON CONFLICT(source_device_id,target_device_id) DO UPDATE SET connection_type=excluded.connection_type,port_label=excluded.port_label,source_type=excluded.source_type,confidence=excluded.confidence,updated_at=excluded.updated_at WHERE connections.user_confirmed=0`, sourceID, targetID, connectionType, port, sourceType, confidence, now(), now())
	if err != nil {
		return false, fmt.Errorf("保存 LLDP 连接: %w", err)
	}
	count, _ := result.RowsAffected()
	return count > 0, nil
}
