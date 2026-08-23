package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type backupInfo struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

func settingInt(settings map[string]string, key string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(settings[key]))
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}

func backupInterval(settings map[string]string) time.Duration {
	value, err := time.ParseDuration(settings["automatic_backup_interval"])
	if err != nil || value < 6*time.Hour || value > 7*24*time.Hour {
		return 24 * time.Hour
	}
	return value
}

func (s *Server) maintain() {
	s.runMaintenance(time.Now())
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for current := range ticker.C {
		s.runMaintenance(current)
	}
}

func (s *Server) runMaintenance(current time.Time) {
	settings, err := s.store.settings()
	if err != nil {
		return
	}
	days := settingInt(settings, "history_retention_days", 30, 7, 365)
	cutoff := current.UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	_, _ = s.store.db.Exec(`DELETE FROM device_samples WHERE checked_at < ?`, cutoff)
	_, _ = s.store.db.Exec(`DELETE FROM status_events WHERE created_at < ?`, cutoff)
	_, _ = s.store.db.Exec(`DELETE FROM scan_runs WHERE finished_at<>'' AND finished_at < ?`, cutoff)
	if !settingEnabled(settings, "automatic_backup_enabled", false) {
		return
	}
	backups, _ := s.automaticBackups()
	if len(backups) == 0 || current.Sub(parseBackupTime(backups[0])) >= backupInterval(settings) {
		_, _ = s.makeAutomaticBackup(current)
		backups, _ = s.automaticBackups()
	}
	s.trimAutomaticBackups(backups, settingInt(settings, "automatic_backup_keep", 7, 1, 30))
}

func parseBackupTime(info backupInfo) time.Time {
	value, _ := time.Parse(time.RFC3339, info.CreatedAt)
	return value
}
func (s *Server) backupDir() string { return filepath.Join(s.cfg.DataDir, "backups") }
func (s *Server) trimAutomaticBackups(backups []backupInfo, keep int) {
	if len(backups) <= keep {
		return
	}
	for _, old := range backups[keep:] {
		_ = os.Remove(filepath.Join(s.backupDir(), old.Name))
	}
}

func (s *Server) automaticBackups() ([]backupInfo, error) {
	entries, err := os.ReadDir(s.backupDir())
	if os.IsNotExist(err) {
		return []backupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]backupInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "meowtopo-auto-") || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		if stat, statErr := entry.Info(); statErr == nil {
			items = append(items, backupInfo{Name: entry.Name(), Size: stat.Size(), CreatedAt: stat.ModTime().UTC().Format(time.RFC3339)})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	return items, nil
}

func (s *Server) makeAutomaticBackup(current time.Time) (backupInfo, error) {
	if err := os.MkdirAll(s.backupDir(), 0750); err != nil {
		return backupInfo{}, err
	}
	name := "meowtopo-auto-" + current.UTC().Format("20060102-150405") + ".zip"
	path, temporary := filepath.Join(s.backupDir(), name), filepath.Join(s.backupDir(), name+".tmp")
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return backupInfo{}, err
	}
	err = s.writeBackup(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return backupInfo{}, err
	}
	if err = os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return backupInfo{}, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return backupInfo{}, err
	}
	return backupInfo{Name: name, Size: stat.Size(), CreatedAt: stat.ModTime().UTC().Format(time.RFC3339)}, nil
}

func (s *Server) maintenanceStatus(w http.ResponseWriter, r *http.Request) {
	settings, _ := s.store.settings()
	backups, err := s.automaticBackups()
	if err != nil {
		fail(w, 500, "backup_failed", err.Error())
		return
	}
	jsonOut(w, 200, map[string]any{"automatic_backup_enabled": settingEnabled(settings, "automatic_backup_enabled", false), "backup_count": len(backups), "backups": backups, "history_retention_days": settingInt(settings, "history_retention_days", 30, 7, 365)})
}

func (s *Server) createAutomaticBackup(w http.ResponseWriter, r *http.Request) {
	info, err := s.makeAutomaticBackup(time.Now())
	if err != nil {
		fail(w, 500, "backup_failed", fmt.Sprintf("创建备份失败：%v", err))
		return
	}
	settings, _ := s.store.settings()
	backups, _ := s.automaticBackups()
	s.trimAutomaticBackups(backups, settingInt(settings, "automatic_backup_keep", 7, 1, 30))
	jsonOut(w, 201, info)
}
