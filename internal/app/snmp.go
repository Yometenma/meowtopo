package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	oidSysDescr        = ".1.3.6.1.2.1.1.1.0"
	oidSysName         = ".1.3.6.1.2.1.1.5.0"
	oidLLDPLocPortDesc = ".1.0.8802.1.1.2.1.3.7.1.4"
	oidLLDPRemChassis  = ".1.0.8802.1.1.2.1.4.1.1.5"
	oidLLDPRemPort     = ".1.0.8802.1.1.2.1.4.1.1.7"
	oidLLDPRemPortDesc = ".1.0.8802.1.1.2.1.4.1.1.8"
	oidLLDPRemSysName  = ".1.0.8802.1.1.2.1.4.1.1.9"
)

type SNMPTarget struct {
	ID              int64  `json:"id"`
	Address         string `json:"address"`
	Port            uint16 `json:"port"`
	Version         string `json:"version"`
	SecurityLevel   string `json:"security_level"`
	Username        string `json:"username"`
	AuthProtocol    string `json:"auth_protocol"`
	PrivacyProtocol string `json:"privacy_protocol"`
	CommunitySet    bool   `json:"community_configured"`
	AuthPasswordSet bool   `json:"auth_password_configured"`
	PrivPasswordSet bool   `json:"privacy_password_configured"`
	Enabled         bool   `json:"enabled"`
	LastStatus      string `json:"last_status"`
	LastError       string `json:"last_error"`
	LastPolledAt    string `json:"last_polled_at"`
}

type snmpTargetSecret struct {
	SNMPTarget
	Community, AuthPassword, PrivacyPassword string
}

type SNMPStatus struct {
	Running      bool   `json:"running"`
	Targets      int    `json:"targets"`
	Successful   int    `json:"successful"`
	Neighbors    int    `json:"neighbors"`
	Connections  int    `json:"connections"`
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
	ErrorSummary string `json:"error_summary,omitempty"`
}

type SNMPDiscovery struct {
	mu     sync.RWMutex
	store  *Store
	status SNMPStatus
}

type lldpNeighbor struct {
	ChassisID, PortID, PortDescription, SystemName string
	LocalPort                                      int
}

func newSNMPDiscovery(store *Store) *SNMPDiscovery { return &SNMPDiscovery{store: store} }

func (d *SNMPDiscovery) Status() SNMPStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *SNMPDiscovery) Poll(ctx context.Context) error {
	d.mu.Lock()
	if d.status.Running {
		d.mu.Unlock()
		return errors.New("SNMP 与 LLDP 读取正在进行")
	}
	targets, err := d.store.snmpTargetSecrets()
	if err != nil {
		d.mu.Unlock()
		return err
	}
	enabledTargets := 0
	for _, target := range targets {
		if target.Enabled {
			enabledTargets++
		}
	}
	status := SNMPStatus{Running: true, Targets: enabledTargets, StartedAt: now()}
	d.status = status
	d.mu.Unlock()

	var failures []string
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		result, pollErr := d.pollTarget(ctx, target)
		if pollErr != nil {
			failures = append(failures, target.Address+": "+pollErr.Error())
			_ = d.store.updateSNMPTargetStatus(target.ID, "failed", pollErr.Error())
			continue
		}
		status.Successful++
		status.Neighbors += result.neighbors
		status.Connections += result.connections
		if result.warning != "" {
			_ = d.store.updateSNMPTargetStatus(target.ID, "partial", result.warning)
		} else {
			_ = d.store.updateSNMPTargetStatus(target.ID, "ok", "")
		}
	}
	status.Running = false
	status.FinishedAt = now()
	status.ErrorSummary = strings.Join(failures, "; ")
	d.mu.Lock()
	d.status = status
	d.mu.Unlock()
	if len(failures) > 0 && status.Successful == 0 {
		return errors.New(status.ErrorSummary)
	}
	return nil
}

type snmpPollResult struct {
	neighbors, connections int
	warning                string
}

func (d *SNMPDiscovery) pollTarget(ctx context.Context, target snmpTargetSecret) (snmpPollResult, error) {
	client, err := makeSNMPClient(target)
	if err != nil {
		return snmpPollResult{}, err
	}
	if err = client.Connect(); err != nil {
		return snmpPollResult{}, fmt.Errorf("连接失败: %w", err)
	}
	defer client.Conn.Close()
	select {
	case <-ctx.Done():
		return snmpPollResult{}, ctx.Err()
	default:
	}
	packet, err := client.Get([]string{oidSysName, oidSysDescr})
	if err != nil || packet == nil || len(packet.Variables) < 2 {
		if err == nil {
			err = errors.New("返回内容不完整")
		}
		return snmpPollResult{}, fmt.Errorf("读取系统信息失败: %w", err)
	}
	name := pduText(packet.Variables[0])
	description := pduText(packet.Variables[1])
	source, err := d.store.upsertSeen(Discovery{IP: target.Address, Hostname: name, Type: managedDeviceType(description), ProbeMethod: "snmp", TypeSource: "snmp", TypeConfidence: .9, TypeEvidence: compactStrings("SNMP system name: "+name, "SNMP system description: "+description)})
	if err != nil {
		return snmpPollResult{}, err
	}
	neighbors, err := readLLDPNeighbors(client)
	if err != nil {
		return snmpPollResult{warning: "SNMP 可用，但设备未提供可读取的 LLDP 邻居表: " + err.Error()}, nil
	}
	portNames := map[int]string{}
	_ = walkSNMP(client, oidLLDPLocPortDesc, func(pdu gosnmp.SnmpPDU) error {
		if index, ok := oidLastInt(pdu.Name); ok {
			portNames[index] = pduText(pdu)
		}
		return nil
	})
	connections := 0
	for _, neighbor := range neighbors {
		targetDevice, matchErr := d.store.matchLLDPNeighbor(neighbor.ChassisID, neighbor.SystemName)
		if matchErr != nil || targetDevice.ID == 0 || targetDevice.ID == source.ID {
			continue
		}
		port := firstNonEmpty(portNames[neighbor.LocalPort], neighbor.PortDescription, neighbor.PortID)
		created, linkErr := d.store.upsertDiscoveredConnection(source.ID, targetDevice.ID, "ethernet", port, "lldp", .95)
		if linkErr == nil && created {
			connections++
		}
	}
	return snmpPollResult{neighbors: len(neighbors), connections: connections}, nil
}

func makeSNMPClient(target snmpTargetSecret) (*gosnmp.GoSNMP, error) {
	ip := net.ParseIP(strings.TrimSpace(target.Address))
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
		return nil, errors.New("SNMP 目标必须是私有 IPv4 地址")
	}
	if target.Port == 0 {
		target.Port = 161
	}
	client := &gosnmp.GoSNMP{Target: ip.String(), Port: target.Port, Timeout: 2 * time.Second, Retries: 1, MaxOids: gosnmp.MaxOids}
	switch target.Version {
	case "1":
		client.Version, client.Community = gosnmp.Version1, target.Community
	case "2c", "":
		client.Version, client.Community = gosnmp.Version2c, target.Community
	case "3":
		flags, err := snmpV3Flags(target.SecurityLevel)
		if err != nil {
			return nil, err
		}
		client.Version, client.MsgFlags, client.SecurityModel = gosnmp.Version3, flags, gosnmp.UserSecurityModel
		client.SecurityParameters = &gosnmp.UsmSecurityParameters{UserName: target.Username, AuthenticationProtocol: snmpAuthProtocol(target.AuthProtocol), AuthenticationPassphrase: target.AuthPassword, PrivacyProtocol: snmpPrivacyProtocol(target.PrivacyProtocol), PrivacyPassphrase: target.PrivacyPassword}
	default:
		return nil, errors.New("不支持的 SNMP 版本")
	}
	return client, validateSNMPTarget(target)
}

func validateSNMPTarget(target snmpTargetSecret) error {
	ip := net.ParseIP(strings.TrimSpace(target.Address))
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
		return errors.New("SNMP 目标必须是私有 IPv4 地址")
	}
	if target.Port == 0 {
		target.Port = 161
	}
	if target.Version == "1" || target.Version == "2c" || target.Version == "" {
		if target.Community == "" || len(target.Community) > 128 {
			return errors.New("community 长度必须为 1 到 128 个字符")
		}
		return nil
	}
	if target.Version != "3" || strings.TrimSpace(target.Username) == "" {
		return errors.New("SNMP v3 必须填写用户名")
	}
	if _, err := snmpV3Flags(target.SecurityLevel); err != nil {
		return err
	}
	if target.SecurityLevel == "authNoPriv" || target.SecurityLevel == "authPriv" {
		if len(target.AuthPassword) < 8 {
			return errors.New("SNMP v3 认证密码至少需要 8 个字符")
		}
		if snmpAuthProtocol(target.AuthProtocol) == gosnmp.NoAuth {
			return errors.New("SNMP v3 认证算法无效")
		}
	}
	if target.SecurityLevel == "authPriv" && len(target.PrivacyPassword) < 8 {
		return errors.New("SNMP v3 加密密码至少需要 8 个字符")
	}
	if target.SecurityLevel == "authPriv" && snmpPrivacyProtocol(target.PrivacyProtocol) == gosnmp.NoPriv {
		return errors.New("SNMP v3 加密算法无效")
	}
	return nil
}

func snmpV3Flags(level string) (gosnmp.SnmpV3MsgFlags, error) {
	switch level {
	case "noAuthNoPriv", "":
		return gosnmp.NoAuthNoPriv, nil
	case "authNoPriv":
		return gosnmp.AuthNoPriv, nil
	case "authPriv":
		return gosnmp.AuthPriv, nil
	default:
		return 0, errors.New("SNMP v3 安全级别无效")
	}
}

func snmpAuthProtocol(protocol string) gosnmp.SnmpV3AuthProtocol {
	return map[string]gosnmp.SnmpV3AuthProtocol{"MD5": gosnmp.MD5, "SHA": gosnmp.SHA, "SHA224": gosnmp.SHA224, "SHA256": gosnmp.SHA256, "SHA384": gosnmp.SHA384, "SHA512": gosnmp.SHA512}[protocol]
}

func snmpPrivacyProtocol(protocol string) gosnmp.SnmpV3PrivProtocol {
	return map[string]gosnmp.SnmpV3PrivProtocol{"DES": gosnmp.DES, "AES": gosnmp.AES, "AES192": gosnmp.AES192, "AES256": gosnmp.AES256, "AES192C": gosnmp.AES192C, "AES256C": gosnmp.AES256C}[protocol]
}

func walkSNMP(client *gosnmp.GoSNMP, root string, fn gosnmp.WalkFunc) error {
	if client.Version == gosnmp.Version1 {
		return client.Walk(root, fn)
	}
	return client.BulkWalk(root, fn)
}

func readLLDPNeighbors(client *gosnmp.GoSNMP) ([]lldpNeighbor, error) {
	tables := []struct {
		oid string
		set func(*lldpNeighbor, gosnmp.SnmpPDU)
	}{
		{oidLLDPRemChassis, func(n *lldpNeighbor, pdu gosnmp.SnmpPDU) { n.ChassisID = formatChassisID(pduBytes(pdu)) }},
		{oidLLDPRemPort, func(n *lldpNeighbor, pdu gosnmp.SnmpPDU) { n.PortID = pduText(pdu) }},
		{oidLLDPRemPortDesc, func(n *lldpNeighbor, pdu gosnmp.SnmpPDU) { n.PortDescription = pduText(pdu) }},
		{oidLLDPRemSysName, func(n *lldpNeighbor, pdu gosnmp.SnmpPDU) { n.SystemName = pduText(pdu) }},
	}
	neighbors := map[string]*lldpNeighbor{}
	for _, table := range tables {
		err := walkSNMP(client, table.oid, func(pdu gosnmp.SnmpPDU) error {
			root := strings.TrimPrefix(table.oid, ".") + "."
			index := strings.TrimPrefix(strings.TrimPrefix(pdu.Name, "."), root)
			parts := strings.Split(index, ".")
			if len(parts) < 3 {
				return nil
			}
			neighbor := neighbors[index]
			if neighbor == nil {
				localPort, _ := strconv.Atoi(parts[len(parts)-2])
				neighbor = &lldpNeighbor{LocalPort: localPort}
				neighbors[index] = neighbor
			}
			table.set(neighbor, pdu)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := make([]lldpNeighbor, 0, len(neighbors))
	for _, neighbor := range neighbors {
		if neighbor.ChassisID != "" || neighbor.SystemName != "" {
			out = append(out, *neighbor)
		}
	}
	return out, nil
}

func pduBytes(pdu gosnmp.SnmpPDU) []byte {
	switch value := pdu.Value.(type) {
	case []byte:
		return value
	case string:
		return []byte(value)
	default:
		return []byte(fmt.Sprint(value))
	}
}

func pduText(pdu gosnmp.SnmpPDU) string { return cleanSNMPText(pduBytes(pdu)) }

func managedDeviceType(description string) string {
	lower := strings.ToLower(description)
	if strings.Contains(lower, "wireless") || strings.Contains(lower, "access point") {
		return "ap"
	}
	if strings.Contains(lower, "router") || strings.Contains(lower, "gateway") {
		return "router"
	}
	return "switch"
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !strings.HasSuffix(value, ":") {
			out = append(out, value)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func oidLastInt(oid string) (int, bool) {
	parts := strings.Split(oid, ".")
	value, err := strconv.Atoi(parts[len(parts)-1])
	return value, err == nil
}

func cleanSNMPText(value []byte) string {
	return strings.TrimSpace(strings.Trim(string(value), "\x00"))
}

func formatChassisID(value []byte) string {
	if len(value) == 6 {
		parts := make([]string, len(value))
		for i, item := range value {
			parts[i] = fmt.Sprintf("%02x", item)
		}
		return strings.Join(parts, ":")
	}
	return cleanSNMPText(value)
}
