package app

import (
	"fmt"
	"time"
)

func (s *Store) devices() (out []Device, err error) {
	r, err := s.db.Query(deviceSelect + ` ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for r.Next() {
		d, e := scanDevice(r)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, r.Err()
}
func (s *Store) device(id int64) (Device, error) {
	return scanDevice(s.db.QueryRow(deviceSelect+` WHERE d.id=?`, id))
}

func (s *Store) createManual(name, typ, notes string) (Device, error) {
	t := now()
	r, e := s.db.Exec(`INSERT INTO devices(stable_key,user_name,user_device_type,notes,first_seen_at,status,is_new,created_manually,created_at,updated_at)VALUES(?,?,?,?,?,'unknown',0,1,?,?)`, fmt.Sprintf("manual:%d-%d", time.Now().UnixNano(), manualSequence.Add(1)), name, typ, notes, t, t, t)
	if e != nil {
		return Device{}, e
	}
	id, _ := r.LastInsertId()
	return s.device(id)
}
