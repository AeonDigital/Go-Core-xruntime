package xruntime_test

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AeonDigital/Go-Core-xruntime/pkg/xruntime"
)

//go:embed appfs/*
var runtimeTestFS embed.FS

func TestNewRuntimeConfig(t *testing.T) {
	rc := xruntime.NewRuntimeConfig()
	if rc == nil {
		t.Fatal("expected non-nil RuntimeConfig")
	}
	if rc.Context == nil {
		t.Error("expected non-nil default Context")
	}
}

func TestStart_And_End_Lifecycle(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "mytestapp"

	// 1. Initial Start: creates FS, parses YAML, compiles JSON, connects DB, creates log
	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("failed to start runtime: %v", err)
	}

	if rc.AppName != appName {
		t.Errorf("expected AppName=%s, got %s", appName, rc.AppName)
	}
	if rc.UserLogDir != filepath.Join(logDir, appName) {
		t.Errorf("expected UserLogDir=%s, got %s", filepath.Join(logDir, appName), rc.UserLogDir)
	}
	if rc.UserDataDir != filepath.Join(dataDir, appName) {
		t.Errorf("expected UserDataDir=%s, got %s", filepath.Join(dataDir, appName), rc.UserDataDir)
	}

	// Verify generated files
	if _, err := os.Stat(rc.UserFileConfigYAML); err != nil {
		t.Errorf("expected config.yaml to exist: %v", err)
	}
	if _, err := os.Stat(rc.UserFileConfigJson); err != nil {
		t.Errorf("expected config.json to exist: %v", err)
	}
	if _, err := os.Stat(rc.UserFileSQLiteData); err != nil {
		t.Errorf("expected sqlite.db to exist: %v", err)
	}
	if _, err := os.Stat(rc.UserFileSessionLog); err != nil {
		t.Errorf("expected current.log to exist: %v", err)
	}

	// Verify DB is open and queryable
	if rc.DBConfig.DB == nil {
		t.Fatal("expected DB connection to be initialized")
	}
	if err := rc.DBConfig.DB.Ping(); err != nil {
		t.Errorf("expected DB ping to succeed, got: %v", err)
	}

	// Verify migrations ran
	var tableName string
	row := rc.DBConfig.DB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sample_items'")
	if err := row.Scan(&tableName); err != nil {
		t.Errorf("expected sample_items table to exist in SQLite DB: %v", err)
	}

	// 2. End gracefully
	if err := rc.End(); err != nil {
		t.Fatalf("expected clean End(), got err: %v", err)
	}

	// 3. Second Start: should load from existing JSON cache
	rc2, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("failed second start from cache: %v", err)
	}
	defer func() {
		_ = xruntime.End(rc2)
	}()

	if rc2.AppName != appName {
		t.Errorf("expected AppName=%s on second start, got %s", appName, rc2.AppName)
	}
}

func TestStart_OutdatedJsonCache_Refreshes(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "cachetest"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("initial start failed: %v", err)
	}
	_ = rc.End()

	// Touch YAML to be newer than JSON cache
	yamlPath := rc.UserFileConfigYAML
	jsonPath := rc.UserFileConfigJson

	oldTime := time.Now().Add(-1 * time.Hour)
	newTime := time.Now()

	if err := os.Chtimes(jsonPath, oldTime, oldTime); err != nil {
		t.Fatalf("failed to chtimes on json cache: %v", err)
	}
	if err := os.Chtimes(yamlPath, newTime, newTime); err != nil {
		t.Fatalf("failed to chtimes on yaml: %v", err)
	}

	// Start again: should invalidate outdated JSON and recompile
	rc2, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("second start after invalidation failed: %v", err)
	}
	defer func() {
		_ = rc2.End()
	}()

	if _, err := os.Stat(rc2.UserFileConfigJson); err != nil {
		t.Errorf("expected regenerated config.json to exist: %v", err)
	}
}

func TestStart_EmptyCorruptedJsonCache_ReturnsError(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "corrupttest"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("initial start failed: %v", err)
	}
	_ = rc.End()

	// Truncate json cache to 0 bytes
	jsonPath := rc.UserFileConfigJson
	if err := os.WriteFile(jsonPath, []byte{}, 0o600); err != nil {
		t.Fatalf("failed to truncate json cache: %v", err)
	}

	// Starting with empty config.json must return error and clean up the corrupted file
	rc2, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err == nil {
		_ = rc2.End()
		t.Fatal("expected error on empty config.json, got nil")
	}

	// Verify corrupted file was removed
	if _, statErr := os.Stat(jsonPath); !os.IsNotExist(statErr) {
		t.Errorf("expected empty corrupted config.json to be deleted")
	}
}

func TestEnd_NilOrDisabledLogging(t *testing.T) {
	// Case 1: nil config
	if err := xruntime.End(nil); err != nil {
		t.Errorf("expected nil error on nil RuntimeConfig, got: %v", err)
	}

	// Case 2: LogRegistry disabled
	rc := xruntime.NewRuntimeConfig()
	rc.Logging.LogRegistry = false
	if err := rc.End(); err != nil {
		t.Errorf("expected nil error when LogRegistry is disabled, got: %v", err)
	}
}

func TestEnd_LogRotationBySize(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "rotatetest"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Configure tiny max size to force rotation
	rc.Logging.LogRegistryFileMaxSize = 10 // 10 bytes

	// Write more than 10 bytes into current.log
	if _, err := rc.CurrentFileSessionLog.WriteString("Long log line to trigger rotation\n"); err != nil {
		t.Fatalf("failed to write log: %v", err)
	}

	if err := rc.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify archived log exists in logDir and current.log was deleted
	entries, err := os.ReadDir(rc.UserLogDir)
	if err != nil {
		t.Fatalf("failed to read logDir: %v", err)
	}

	hasArchivedLog := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".log") && entry.Name() != xruntime.FileSessionLog {
			hasArchivedLog = true
			break
		}
	}

	if !hasArchivedLog {
		t.Errorf("expected rotated archive .log file in %s, found: %v", rc.UserLogDir, entries)
	}
}

func TestStart_DefaultDirectoriesFallback(t *testing.T) {
	appName := "defaultdirtester"

	// Passing empty logDir and dataDir will test XDG user default directory resolution
	rc, err := xruntime.Start(appName, runtimeTestFS, "", "")
	if err != nil {
		t.Fatalf("start with default dirs failed: %v", err)
	}
	defer func() {
		_ = rc.End()
		// Clean up user dirs created for test
		_ = os.RemoveAll(rc.UserDataDir)
		_ = os.RemoveAll(rc.UserLogDir)
	}()

	if !strings.HasSuffix(rc.UserDataDir, appName) {
		t.Errorf("expected UserDataDir to end with %s, got %s", appName, rc.UserDataDir)
	}
	if !strings.HasSuffix(rc.UserLogDir, appName) {
		t.Errorf("expected UserLogDir to end with %s, got %s", appName, rc.UserLogDir)
	}
}
