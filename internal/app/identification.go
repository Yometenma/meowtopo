package app

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

type identificationEvidence struct {
	Kind       string
	Value      string
	DeviceType string
	Weight     float64
}

type identificationResult struct {
	DeviceType string
	Source     string
	Confidence float64
	Evidence   []string
}

type hostnameRule struct {
	DeviceType string
	Confidence float64
	Words      []string
}

var deviceHostnameRules = []hostnameRule{
	{"nas", .92, []string{"synology", "diskstation", "qnap", "truenas", "openmediavault", "asustor", "-nas", "nas-"}},
	{"ap", .89, []string{"access-point", "wireless-ap", "unifi-ap", "uap-", "omada-ap"}},
	{"router", .9, []string{"openwrt", "opnsense", "pfsense", "router", "gateway"}},
	{"switch", .89, []string{"switch", "tl-sg", "tl-sl", "-sw-", "sw-core", "core-sw"}},
	{"camera", .88, []string{"ipcam", "camera", "webcam", "nvr", "dvr"}},
	{"printer", .86, []string{"printer", "laserjet", "deskjet", "officejet"}},
	{"iot", .84, []string{"homeassistant", "home-assistant", "esphome", "shelly", "tuya", "tasmota", "xiaomi", "yeelight", "aqara", "hue-bridge"}},
	{"linux", .78, []string{"proxmox", "pve", "esxi", "server", "ubuntu", "debian", "raspberrypi", "raspberry-pi"}},
	{"windows", .76, []string{"desktop-", "laptop-", "win-pc", "surface"}},
	{"macos", .76, []string{"macbook", "imac", "mac-mini"}},
	{"tablet", .76, []string{"ipad", "tablet"}},
	{"phone", .72, []string{"iphone", "android", "pixel", "phone", "galaxy", "huawei", "oneplus"}},
	{"tv", .7, []string{"smarttv", "androidtv", "appletv", "chromecast", "firetv", "roku"}},
	{"game", .74, []string{"playstation", "xbox", "nintendo", "steamdeck"}},
}

func identifyDevice(host, mac string, ports []int, extra ...identificationEvidence) identificationResult {
	evidence := hostnameEvidence(host)
	evidence = append(evidence, portEvidence(ports)...)
	evidence = append(evidence, extra...)
	result := combineIdentificationEvidence(evidence)
	if isLocallyAdministeredMAC(mac) {
		result.Evidence = append(result.Evidence, "MAC：随机或本地管理地址，不用于判断厂商")
	}
	return result
}

func hostnameEvidence(host string) []identificationEvidence {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, rule := range deviceHostnameRules {
		for _, word := range rule.Words {
			if strings.Contains(host, word) {
				return []identificationEvidence{{Kind: "hostname", Value: host, DeviceType: rule.DeviceType, Weight: rule.Confidence}}
			}
		}
	}
	return nil
}

func portEvidence(ports []int) []identificationEvidence {
	open := map[int]bool{}
	for _, port := range ports {
		open[port] = true
	}
	var evidence []identificationEvidence
	add := func(deviceType string, weight float64, description string) {
		evidence = append(evidence, identificationEvidence{Kind: "ports", Value: description, DeviceType: deviceType, Weight: weight})
	}
	if open[8123] {
		add("iot", .78, "8123（Home Assistant）")
	}
	if open[9100] {
		add("printer", .82, "9100（网络打印）")
	}
	if open[631] {
		add("printer", .68, "631（IPP）")
	}
	if open[62078] {
		add("phone", .76, "62078（Apple 设备同步）")
	}
	if open[554] {
		add("camera", .72, "554（RTSP）")
	}
	if open[5357] {
		add("windows", .7, "5357（Windows 设备发现）")
	}
	if open[548] {
		add("macos", .7, "548（AFP）")
	}
	if open[8008] {
		add("tv", .66, "8008（媒体设备）")
	}
	if open[3389] {
		add("windows", .68, "3389（远程桌面）")
	}
	if open[32400] {
		add("linux", .5, "32400（Plex）")
	}
	if open[53] && (open[80] || open[443]) {
		add("router", .6, "DNS 与管理页面组合")
	}
	if open[5000] && (open[445] || open[22]) {
		add("nas", .72, "存储服务端口组合")
	}
	if open[22] {
		add("linux", .45, "22（SSH，弱线索）")
	}
	return evidence
}

func combineIdentificationEvidence(evidence []identificationEvidence) identificationResult {
	if len(evidence) == 0 {
		return identificationResult{DeviceType: "unknown"}
	}
	type score struct {
		total, strongest float64
		count            int
	}
	scores := map[string]score{}
	for _, item := range evidence {
		current := scores[item.DeviceType]
		current.total += item.Weight
		current.count++
		if item.Weight > current.strongest {
			current.strongest = item.Weight
		}
		scores[item.DeviceType] = current
	}
	types := make([]string, 0, len(scores))
	for deviceType := range scores {
		types = append(types, deviceType)
	}
	sort.Slice(types, func(i, j int) bool {
		a, b := scores[types[i]], scores[types[j]]
		if a.total == b.total {
			return a.strongest > b.strongest
		}
		return a.total > b.total
	})
	winner := types[0]
	winning := scores[winner]
	confidence := winning.strongest
	if winning.count > 1 {
		confidence += .06 * float64(winning.count-1)
	}
	if len(types) > 1 && scores[types[1]].total >= winning.total*.8 {
		confidence -= .15
	}
	if confidence > .98 {
		confidence = .98
	}
	if confidence < 0 {
		confidence = 0
	}

	sourceSet := map[string]bool{}
	labels := make([]string, 0, len(evidence))
	for _, item := range evidence {
		sourceSet[item.Kind] = true
		prefix := map[string]string{"hostname": "主机名", "ports": "开放端口", "vendor": "MAC 厂商", "local_corrections": "本地修正"}[item.Kind]
		if prefix == "" {
			prefix = "识别线索"
		}
		labels = append(labels, fmt.Sprintf("%s：%s → %s", prefix, item.Value, deviceTypeName(item.DeviceType)))
	}
	sources := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return identificationResult{DeviceType: winner, Source: strings.Join(sources, "+"), Confidence: confidence, Evidence: labels}
}

func deviceTypeName(deviceType string) string {
	if label := map[string]string{
		"nas": "NAS", "ap": "无线接入点", "router": "路由器", "switch": "交换机",
		"camera": "摄像头", "printer": "打印机", "iot": "智能家居", "linux": "Linux 设备",
		"windows": "Windows 电脑", "macos": "Mac", "tablet": "平板", "phone": "手机",
		"tv": "电视/媒体设备", "game": "游戏设备",
	}[deviceType]; label != "" {
		return label
	}
	return "未知设备"
}

func identifyType(host string, ports []int) (string, string, float64) {
	result := identifyDevice(host, "", ports)
	return result.DeviceType, result.Source, result.Confidence
}

func isLocallyAdministeredMAC(mac string) bool {
	hardware, err := net.ParseMAC(mac)
	return err == nil && len(hardware) >= 1 && hardware[0]&0x02 != 0
}

func identifyVendor(host, mac string) string {
	if isLocallyAdministeredMAC(mac) {
		return ""
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	rules := []struct {
		name  string
		words []string
	}{
		{"Synology", []string{"synology", "diskstation"}}, {"QNAP", []string{"qnap"}},
		{"TrueNAS", []string{"truenas"}}, {"ASUSTOR", []string{"asustor"}},
		{"TP-Link", []string{"tp-link", "tplink", "tl-sg", "tl-sl", "deco"}},
		{"Ubiquiti", []string{"ubiquiti", "unifi", "uap-"}},
		{"Xiaomi", []string{"xiaomi", "miwifi", "yeelight", "aqara"}},
		{"Apple", []string{"iphone", "ipad", "macbook", "imac", "mac-mini", "appletv", "homepod"}},
		{"Google", []string{"chromecast", "google-home", "nest-"}}, {"Amazon", []string{"firetv", "echo-"}},
		{"Samsung", []string{"samsung", "galaxy"}}, {"Raspberry Pi", []string{"raspberrypi", "raspberry-pi"}},
	}
	for _, rule := range rules {
		for _, word := range rule.words {
			if strings.Contains(host, word) {
				return rule.name
			}
		}
	}
	return ""
}
