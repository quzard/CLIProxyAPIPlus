package usage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const defaultStatisticsPersistenceFile = ".usage_statistics.json"

// StatisticsPersistencePayload is the on-disk usage statistics snapshot format.
type StatisticsPersistencePayload struct {
	Version    int                `json:"version"`
	ExportedAt time.Time          `json:"exported_at"`
	Usage      StatisticsSnapshot `json:"usage"`
}

// ResolveStatisticsPersistencePath returns the effective persistence path.
func ResolveStatisticsPersistencePath(configPath, configuredPath string, enabled bool) string {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" && !enabled {
		return ""
	}

	baseDir := "."
	if trimmedConfigPath := strings.TrimSpace(configPath); trimmedConfigPath != "" {
		baseDir = filepath.Dir(trimmedConfigPath)
	}

	if configuredPath == "" {
		return filepath.Join(baseDir, defaultStatisticsPersistenceFile)
	}
	if filepath.IsAbs(configuredPath) {
		return configuredPath
	}
	return filepath.Join(baseDir, configuredPath)
}

// StartStatisticsPersistence loads an existing snapshot and periodically stores the current one.
func StartStatisticsPersistence(ctx context.Context, path string, interval time.Duration) func() {
	path = strings.TrimSpace(path)
	if path == "" {
		return func() {}
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if ctx == nil {
		ctx = context.Background()
	}

	stats := GetRequestStatistics()
	if err := loadStatisticsSnapshot(path, stats); err != nil {
		log.Warnf("failed to load usage statistics snapshot %s: %v", path, err)
	}

	persistCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var stopOnce sync.Once

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := saveStatisticsSnapshot(path, stats); err != nil {
					log.Warnf("failed to persist usage statistics snapshot %s: %v", path, err)
				}
			case <-persistCtx.Done():
				if err := saveStatisticsSnapshot(path, stats); err != nil {
					log.Warnf("failed to persist usage statistics snapshot during shutdown %s: %v", path, err)
				}
				return
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			cancel()
			<-done
		})
	}
}

func loadStatisticsSnapshot(path string, stats *RequestStatistics) error {
	if stats == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var payload StatisticsPersistencePayload
	if err := json.Unmarshal(data, &payload); err == nil && !statisticsSnapshotEmpty(payload.Usage) {
		result := stats.MergeSnapshot(payload.Usage)
		log.Infof("loaded usage statistics snapshot from %s (added=%d skipped=%d)", path, result.Added, result.Skipped)
		return nil
	}

	var snapshot StatisticsSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	if statisticsSnapshotEmpty(snapshot) {
		return nil
	}
	result := stats.MergeSnapshot(snapshot)
	log.Infof("loaded usage statistics snapshot from %s (added=%d skipped=%d)", path, result.Added, result.Skipped)
	return nil
}

func saveStatisticsSnapshot(path string, stats *RequestStatistics) error {
	if stats == nil {
		return nil
	}
	snapshot := stats.Snapshot()
	if statisticsSnapshotEmpty(snapshot) {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".usage_statistics_*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(StatisticsPersistencePayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	}); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func statisticsSnapshotEmpty(snapshot StatisticsSnapshot) bool {
	if snapshot.TotalRequests > 0 || snapshot.SuccessCount > 0 || snapshot.FailureCount > 0 || snapshot.TotalTokens > 0 {
		return false
	}
	return len(snapshot.APIs) == 0 &&
		len(snapshot.RequestsByDay) == 0 &&
		len(snapshot.RequestsByHour) == 0 &&
		len(snapshot.TokensByDay) == 0 &&
		len(snapshot.TokensByHour) == 0
}
