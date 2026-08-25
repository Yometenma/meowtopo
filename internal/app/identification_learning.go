package app

import "fmt"

type learnedIdentificationRule struct {
	DeviceType string
	Count      int
	Total      int
}

func identificationEvidenceFor(vendor string, learned map[string]learnedIdentificationRule, service []identificationEvidence) []identificationEvidence {
	evidence := append([]identificationEvidence{}, service...)
	evidence = append(evidence, vendorEvidence(vendor)...)
	if rule, ok := learned[normalizeVendor(vendor)]; ok {
		evidence = append(evidence, identificationEvidence{
			Kind:       "local_corrections",
			Value:      fmt.Sprintf("同厂商 %d/%d 台设备", rule.Count, rule.Total),
			DeviceType: rule.DeviceType,
			Weight:     .66,
		})
	}
	return evidence
}

// learnedVendorRules uses only the latest correction from each device. One
// repeatedly edited device therefore cannot train a rule by itself.
func (s *Store) learnedVendorRules() (map[string]learnedIdentificationRule, error) {
	rows, err := s.db.Query(`SELECT COALESCE(NULLIF(c.vendor,''),d.vendor),c.corrected_type
		FROM identification_corrections c JOIN devices d ON d.id=c.device_id
		WHERE c.id=(SELECT MAX(latest.id) FROM identification_corrections latest WHERE latest.device_id=c.device_id)
		AND d.user_device_type<>'' AND d.user_device_type<>d.auto_device_type AND c.corrected_type=d.user_device_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type correctionCounts struct {
		total  int
		byType map[string]int
	}
	grouped := map[string]*correctionCounts{}
	for rows.Next() {
		var vendor, deviceType string
		if err = rows.Scan(&vendor, &deviceType); err != nil {
			return nil, err
		}
		vendor = normalizeVendor(vendor)
		if vendor == "" || deviceType == "" {
			continue
		}
		group := grouped[vendor]
		if group == nil {
			group = &correctionCounts{byType: map[string]int{}}
			grouped[vendor] = group
		}
		group.total++
		group.byType[deviceType]++
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rules := map[string]learnedIdentificationRule{}
	for vendor, group := range grouped {
		winner, count := "", 0
		for deviceType, candidate := range group.byType {
			if candidate > count {
				winner, count = deviceType, candidate
			}
		}
		if group.total >= 2 && count*4 >= group.total*3 {
			rules[vendor] = learnedIdentificationRule{DeviceType: winner, Count: count, Total: group.total}
		}
	}
	return rules, nil
}
