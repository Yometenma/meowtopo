package app

import "strings"

type vendorIdentificationRule struct {
	DeviceType string
	Weight     float64
	Names      []string
}

var deviceVendorRules = []vendorIdentificationRule{
	{"nas", .72, []string{"synology", "qnap", "asustor", "ixsystems", "truenas"}},
	{"camera", .68, []string{"hikvision", "dahua", "axis communications", "reolink"}},
	{"printer", .62, []string{"brother industries", "xerox", "lexmark", "kyocera document"}},
	{"linux", .58, []string{"raspberry pi"}},
	{"iot", .62, []string{"espressif", "tuya", "shelly", "signify", "philips lighting"}},
	{"game", .62, []string{"nintendo"}},
	{"tv", .58, []string{"roku"}},
}

// Vendor-only rules are limited to manufacturers whose products strongly
// indicate one category. Broad manufacturers are deliberately omitted.
func vendorEvidence(vendor string) []identificationEvidence {
	normalized := normalizeVendor(vendor)
	if normalized == "" {
		return nil
	}
	for _, rule := range deviceVendorRules {
		for _, name := range rule.Names {
			if strings.Contains(normalized, name) {
				return []identificationEvidence{{Kind: "vendor", Value: vendor, DeviceType: rule.DeviceType, Weight: rule.Weight}}
			}
		}
	}
	return nil
}

func normalizeVendor(vendor string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(vendor))), " ")
}
