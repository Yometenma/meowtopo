package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateSNMPTarget(t *testing.T) {
	public := snmpTargetSecret{SNMPTarget: SNMPTarget{Address: "192.0.2.10", Version: "2c"}, Community: "readonly"}
	if err := validateSNMPTarget(public); err == nil {
		t.Fatal("public address accepted as SNMP target")
	}
	valid := []snmpTargetSecret{
		{SNMPTarget: SNMPTarget{Address: "192.168.1.2", Version: "2c"}, Community: "readonly"},
		{SNMPTarget: SNMPTarget{Address: "10.20.30.40", Version: "3", Username: "monitor", SecurityLevel: "noAuthNoPriv"}},
		{SNMPTarget: SNMPTarget{Address: "172.20.1.2", Version: "3", Username: "monitor", SecurityLevel: "authPriv", AuthProtocol: "SHA256", PrivacyProtocol: "AES"}, AuthPassword: "auth-pass", PrivacyPassword: "priv-pass"},
	}
	for _, target := range valid {
		if err := validateSNMPTarget(target); err != nil {
			t.Fatalf("valid target rejected: %v", err)
		}
	}
	bad := snmpTargetSecret{SNMPTarget: SNMPTarget{Address: "192.168.1.2", Version: "3", Username: "monitor", SecurityLevel: "authPriv"}, AuthPassword: "short", PrivacyPassword: "short"}
	if err := validateSNMPTarget(bad); err == nil {
		t.Fatal("short SNMPv3 secrets accepted")
	}
	bad.SecurityLevel = "unexpected"
	if err := validateSNMPTarget(bad); err == nil {
		t.Fatal("unknown SNMPv3 security level accepted")
	}
}

func TestSNMPTargetHandlersNeverReturnSecrets(t *testing.T) {
	server := testServer(t)
	create := httptest.NewRecorder()
	server.createSNMPTarget(create, httptest.NewRequest(http.MethodPost, "/api/snmp/targets", bytes.NewBufferString(`{"address":"192.168.60.2","port":161,"version":"2c","community":"do-not-return","security_level":"noAuthNoPriv","auth_protocol":"SHA256","privacy_protocol":"AES","enabled":true}`)))
	if create.Code != http.StatusCreated || strings.Contains(create.Body.String(), "do-not-return") {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	server.listSNMPTargets(list, httptest.NewRequest(http.MethodGet, "/api/snmp/targets", nil))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "do-not-return") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var response struct {
		Targets []SNMPTarget `json:"targets"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &response); err != nil || len(response.Targets) != 1 || !response.Targets[0].CommunitySet {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestSNMPTargetStoreMasksAndPreservesSecrets(t *testing.T) {
	store := testStore(t)
	target, err := store.saveSNMPTarget(snmpTargetSecret{SNMPTarget: SNMPTarget{Address: "192.168.50.2", Port: 161, Version: "2c", Enabled: true}, Community: "private-read"})
	if err != nil {
		t.Fatal(err)
	}
	if !target.CommunitySet {
		t.Fatal("stored community was not reported as configured")
	}
	listed, err := store.snmpTargets()
	if err != nil || len(listed) != 1 || !listed[0].CommunitySet {
		t.Fatalf("masked target list=%+v err=%v", listed, err)
	}
	updated, err := store.saveSNMPTarget(snmpTargetSecret{SNMPTarget: SNMPTarget{ID: target.ID, Address: "192.168.50.2", Port: 1161, Version: "2c", Enabled: true}})
	if err != nil || updated.Port != 1161 {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	secret, _ := store.snmpTargetSecret(target.ID)
	if secret.Community != "private-read" {
		t.Fatal("blank update erased the saved secret")
	}
	if err = store.deleteSNMPTarget(target.ID); err != nil {
		t.Fatal(err)
	}
}

func TestLLDPConnectionDoesNotOverwriteManualRelationship(t *testing.T) {
	store := testStore(t)
	a, _ := store.createManual("Managed switch", "switch", "")
	b, _ := store.createManual("Server", "server", "")
	_, err := store.db.Exec(`INSERT INTO connections(source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed,created_at,updated_at) VALUES(?,?,'logical','manual','manual',1,1,?,?)`, a.ID, b.ID, now(), now())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := store.upsertDiscoveredConnection(a.ID, b.ID, "ethernet", "Gi1/0/8", "lldp", .95)
	if err != nil || changed {
		t.Fatalf("manual connection was changed: changed=%v err=%v", changed, err)
	}
	links, _ := store.connections()
	if len(links) != 1 || links[0].SourceType != "manual" || links[0].Port != "manual" {
		t.Fatalf("manual connection overwritten: %+v", links)
	}
}

func TestSNMPHelpers(t *testing.T) {
	if got := formatChassisID([]byte{0, 17, 34, 51, 68, 85}); got != "00:11:22:33:44:55" {
		t.Fatalf("chassis=%q", got)
	}
	if managedDeviceType("Wireless Access Point") != "ap" || managedDeviceType("Core router") != "router" || managedDeviceType("Ethernet fabric") != "switch" {
		t.Fatal("managed device type classification failed")
	}
	if message := friendlySNMPError(assertError("request timeout")); !strings.Contains(message, "没有响应") {
		t.Fatalf("friendly error=%q", message)
	}
}

func TestMakeSNMPV3Client(t *testing.T) {
	target := snmpTargetSecret{SNMPTarget: SNMPTarget{Address: "10.0.0.2", Port: 1161, Version: "3", Username: "monitor", SecurityLevel: "authPriv", AuthProtocol: "SHA256", PrivacyProtocol: "AES256C"}, AuthPassword: "auth-pass", PrivacyPassword: "priv-pass"}
	client, err := makeSNMPClient(target)
	if err != nil {
		t.Fatal(err)
	}
	if client.Target != "10.0.0.2" || client.Port != 1161 || client.MsgFlags.String() != "AuthPriv" {
		t.Fatalf("unexpected SNMPv3 client: target=%s port=%d flags=%s", client.Target, client.Port, client.MsgFlags)
	}
}

func TestLLDPRemoteIndexParsing(t *testing.T) {
	// Column 5 is lldpRemChassisId; the index is timeMark.localPortNum.remoteIndex.
	index, port, ok := lldpRemoteIndex(".1.0.8802.1.1.2.1.4.1.1.5.0.2.1", 5)
	if !ok || index != "0.2.1" || port != 2 {
		t.Fatalf("chassis index=%q port=%d ok=%v", index, port, ok)
	}
	// The port-description column (8) must group under the same index.
	index, port, ok = lldpRemoteIndex(".1.0.8802.1.1.2.1.4.1.1.8.0.2.1", 8)
	if !ok || index != "0.2.1" || port != 2 {
		t.Fatalf("port-desc index=%q port=%d ok=%v", index, port, ok)
	}
	if _, _, ok := lldpRemoteIndex(".1.0.8802.1.1.2.1.4.1.1.9.0.2.1", 5); ok {
		t.Fatal("wrong column accepted")
	}
	if _, _, ok := lldpRemoteIndex(".1.2.3", 5); ok {
		t.Fatal("truncated OID accepted")
	}
	if _, _, ok := lldpRemoteIndex("not-an-oid", 5); ok {
		t.Fatal("non-numeric OID accepted")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
