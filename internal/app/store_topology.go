package app

func (s *Store) connections() (out []Connection, err error) {
	r, e := s.db.Query(`SELECT id,source_device_id,target_device_id,connection_type,port_label,source_type,confidence,user_confirmed FROM connections`)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	for r.Next() {
		var c Connection
		var b int
		if e = r.Scan(&c.ID, &c.SourceID, &c.TargetID, &c.Type, &c.Port, &c.SourceType, &c.Confidence, &b); e != nil {
			return nil, e
		}
		c.Confirmed = b != 0
		out = append(out, c)
	}
	return out, r.Err()
}
