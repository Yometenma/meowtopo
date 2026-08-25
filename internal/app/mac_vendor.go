package app

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type macVendorDatabase struct {
	mu        sync.RWMutex
	path      string
	entries   map[string]string
	updatedAt time.Time
}

type macVendorStatus struct {
	Available bool   `json:"available"`
	Entries   int    `json:"entries"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Source    string `json:"source"`
}

var ieeeVendorSources = []struct {
	Registry string
	URL      string
}{
	{"MA-L", "https://standards-oui.ieee.org/oui/oui.csv"},
	{"MA-M", "https://standards-oui.ieee.org/oui28/mam.csv"},
	{"MA-S", "https://standards-oui.ieee.org/oui36/oui36.csv"},
}

func openMACVendorDatabase(dataDir string) *macVendorDatabase {
	database := &macVendorDatabase{path: filepath.Join(dataDir, "mac-vendors.tsv"), entries: map[string]string{}}
	_ = database.load()
	return database
}

func (database *macVendorDatabase) Lookup(mac string) string {
	if isLocallyAdministeredMAC(mac) {
		return ""
	}
	normalized := strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "").Replace(mac))
	if len(normalized) < 12 {
		return ""
	}
	database.mu.RLock()
	defer database.mu.RUnlock()
	for _, length := range []int{9, 7, 6} {
		if vendor := database.entries[normalized[:length]]; vendor != "" {
			return vendor
		}
	}
	return ""
}

func (database *macVendorDatabase) Status() macVendorStatus {
	database.mu.RLock()
	defer database.mu.RUnlock()
	status := macVendorStatus{Available: len(database.entries) > 0, Entries: len(database.entries), Source: "IEEE Registration Authority public listings"}
	if !database.updatedAt.IsZero() {
		status.UpdatedAt = database.updatedAt.UTC().Format(time.RFC3339)
	}
	return status
}

func (database *macVendorDatabase) Update(ctx context.Context) error {
	client := &http.Client{Timeout: 30 * time.Second}
	entries := map[string]string{}
	for _, source := range ieeeVendorSources {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("下载 IEEE %s 名单: %w", source.Registry, err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return fmt.Errorf("下载 IEEE %s 名单: HTTP %d", source.Registry, response.StatusCode)
		}
		err = readIEEEVendorCSV(io.LimitReader(response.Body, 32<<20), entries)
		response.Body.Close()
		if err != nil {
			return fmt.Errorf("读取 IEEE %s 名单: %w", source.Registry, err)
		}
	}
	if len(entries) < 1000 {
		return fmt.Errorf("IEEE 厂商名单内容不完整")
	}
	if err := writeVendorDatabase(database.path, entries); err != nil {
		return err
	}
	return database.load()
}

func readIEEEVendorCSV(reader io.Reader, entries map[string]string) error {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	header, err := csvReader.Read()
	if err != nil {
		return err
	}
	assignmentIndex, organizationIndex := -1, -1
	for index, field := range header {
		switch strings.TrimSpace(strings.ToLower(field)) {
		case "assignment":
			assignmentIndex = index
		case "organization name":
			organizationIndex = index
		}
	}
	if assignmentIndex < 0 || organizationIndex < 0 {
		return fmt.Errorf("缺少 Assignment 或 Organization Name 列")
	}
	for {
		record, readErr := csvReader.Read()
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
		if assignmentIndex >= len(record) || organizationIndex >= len(record) {
			continue
		}
		prefix := strings.ToUpper(strings.TrimSpace(record[assignmentIndex]))
		vendor := strings.TrimSpace(record[organizationIndex])
		if (len(prefix) == 6 || len(prefix) == 7 || len(prefix) == 9) && vendor != "" && !strings.EqualFold(vendor, "private") {
			entries[prefix] = vendor
		}
	}
}

func writeVendorDatabase(path string, entries map[string]string) error {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	_, err = writer.WriteString("# IEEE Registration Authority public listings\n")
	for _, key := range keys {
		if err != nil {
			break
		}
		_, err = fmt.Fprintf(writer, "%s\t%s\n", key, strings.ReplaceAll(entries[key], "\t", " "))
	}
	if flushErr := writer.Flush(); err == nil {
		err = flushErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err = os.Rename(temporary, path); err == nil {
		return nil
	}
	// Windows does not replace an existing destination with os.Rename.
	if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
		_ = os.Remove(temporary)
		return removeErr
	}
	if err = os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
	}
	return err
}

func (database *macVendorDatabase) load() error {
	file, err := os.Open(database.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	entries := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			entries[parts[0]] = parts[1]
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	info, _ := file.Stat()
	database.mu.Lock()
	database.entries = entries
	if info != nil {
		database.updatedAt = info.ModTime()
	}
	database.mu.Unlock()
	return nil
}
