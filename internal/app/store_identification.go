package app

import (
	"encoding/json"
)

func (s *Store) recordIdentificationCorrection(deviceID int64, automaticType, correctedType, vendor string, evidence []string) error {
	if correctedType == "" || correctedType == automaticType {
		return nil
	}
	encoded, _ := json.Marshal(evidence)
	_, err := s.db.Exec(`INSERT INTO identification_corrections(device_id,automatic_type,corrected_type,vendor,evidence,created_at) VALUES(?,?,?,?,?,?)`, deviceID, automaticType, correctedType, vendor, string(encoded), now())
	return err
}
