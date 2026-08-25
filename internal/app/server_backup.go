package app

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (s *Server) backup(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="meowtopo-backup.zip"`)
	if err := s.writeBackup(w); err != nil {
		fail(w, http.StatusInternalServerError, "backup_failed", "数据库检查点创建失败")
	}
}

func (s *Server) writeBackup(dst io.Writer) error {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	if _, err := s.store.db.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		return err
	}
	source, err := os.Open(s.store.path)
	if err != nil {
		return err
	}
	defer source.Close()
	archive := zip.NewWriter(dst)
	file, err := archive.Create("meowtopo.db")
	if err == nil {
		_, err = io.Copy(file, source)
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_backup", "备份读取失败")
		return
	}
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_backup", "备份不是有效 ZIP")
		return
	}
	var database []byte
	for _, file := range archive.File {
		if file.Name != "meowtopo.db" || file.UncompressedSize64 > 64<<20 {
			continue
		}
		reader, openErr := file.Open()
		if openErr != nil {
			continue
		}
		database, _ = io.ReadAll(reader)
		reader.Close()
	}
	if len(database) < 16 || string(database[:15]) != "SQLite format 3" {
		fail(w, http.StatusBadRequest, "invalid_backup", "备份不含有效数据库")
		return
	}
	temporary := s.store.path + ".restore"
	if err = os.WriteFile(temporary, database, 0600); err != nil {
		fail(w, http.StatusInternalServerError, "restore_failed", err.Error())
		return
	}
	checked, err := sqlOpenCheck(temporary)
	if err != nil {
		os.Remove(temporary)
		fail(w, http.StatusBadRequest, "invalid_backup", err.Error())
		return
	}
	checked.Close()
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	s.store.db.Close()
	previous := s.store.path + ".pre-restore-" + time.Now().UTC().Format("20060102T150405Z")
	if err = os.Rename(s.store.path, previous); err != nil {
		fail(w, http.StatusInternalServerError, "restore_failed", err.Error())
		return
	}
	if err = os.Rename(temporary, s.store.path); err != nil {
		_ = os.Rename(previous, s.store.path)
		fail(w, http.StatusInternalServerError, "restore_failed", err.Error())
		return
	}
	s.store, err = openStore(filepath.Dir(s.store.path))
	if err != nil {
		fail(w, http.StatusInternalServerError, "restore_failed", err.Error())
		return
	}
	s.notifier = newNotifier(s.store)
	s.snmp = newSNMPDiscovery(s.store)
	s.scanner.store = s.store
	s.scanner.notifier = s.notifier
	s.scanner.snmp = s.snmp
	jsonOut(w, http.StatusOK, map[string]string{"status": "restored", "message": "恢复完成"})
}

func sqlOpenCheck(path string) (io.Closer, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	var result string
	err = database.QueryRow(`PRAGMA integrity_check`).Scan(&result)
	if err != nil || result != "ok" {
		database.Close()
		return nil, fmt.Errorf("数据库完整性检查失败")
	}
	return database, nil
}
