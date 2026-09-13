package xruntime_test

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AeonDigital/Go-Core-xerrors/pkg/xerrors"
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

func TestReset_NilConfig(t *testing.T) {
	err := xruntime.Reset(nil, nil, nil)
	if err != nil {
		t.Errorf("expected nil error for nil runtimeConfig, got: %v", err)
	}
}

func TestReset_PreResetReturnsFalse(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "resettest1"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	preCalled := false
	posCalled := false

	preReset := func() (bool, error) {
		preCalled = true
		return false, nil
	}
	posReset := func() error {
		posCalled = true
		return nil
	}

	err = rc.Reset(preReset, posReset)
	if err == nil {
		t.Fatal("expected error when preReset returns false, got nil")
	}

	cliErr, ok := err.(xerrors.IErrorCLI)
	if !ok {
		t.Fatalf("expected error to implement xerrors.IErrorCLI, got %T", err)
	}

	expectedUserMsg := "could not perform reset due to primary conditions failure"
	if cliErr.GetUserMessage() != expectedUserMsg {
		t.Errorf("expected GetUserMessage() = %q, got %q", expectedUserMsg, cliErr.GetUserMessage())
	}

	if !preCalled {
		t.Error("expected preReset to be called")
	}
	if posCalled {
		t.Error("expected posReset NOT to be called when preReset returns false")
	}

	// Directories must still exist
	if _, statErr := os.Stat(rc.UserDataDir); os.IsNotExist(statErr) {
		t.Error("expected UserDataDir to still exist")
	}
}

func TestReset_PreResetReturnsError(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "resettest2"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	expectedErr := os.ErrPermission

	preReset := func() (bool, error) {
		return false, expectedErr
	}

	err = rc.Reset(preReset, nil)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestReset_SuccessfulResetAndPosReset(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "resettest3"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	userDataDir := rc.UserDataDir
	userLogDir := rc.UserLogDir

	preCalled := false
	posCalled := false

	preReset := func() (bool, error) {
		preCalled = true
		return true, nil
	}
	posReset := func() error {
		posCalled = true
		return nil
	}

	err = rc.Reset(preReset, posReset)
	if err != nil {
		t.Fatalf("expected Reset to succeed, got: %v", err)
	}

	if !preCalled {
		t.Error("expected preReset to be called")
	}
	if !posCalled {
		t.Error("expected posReset to be called")
	}

	// Directories must be deleted
	if _, statErr := os.Stat(userDataDir); !os.IsNotExist(statErr) {
		t.Errorf("expected UserDataDir to be deleted, statErr: %v", statErr)
	}
	if _, statErr := os.Stat(userLogDir); !os.IsNotExist(statErr) {
		t.Errorf("expected UserLogDir to be deleted, statErr: %v", statErr)
	}
}

func TestReset_PosResetReturnsError(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "resettest4"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	expectedErr := os.ErrInvalid
	posReset := func() error {
		return expectedErr
	}

	err = rc.Reset(nil, posReset)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestReset_DeleteUserLogDirError(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "resettest5"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Close log file descriptor before changing dir permissions
	if rc.CurrentFileSessionLog != nil {
		_ = rc.CurrentFileSessionLog.Close()
		rc.CurrentFileSessionLog = nil
	}

	// Make log directory read-only so deleting files inside fails
	if err := os.Chmod(rc.UserLogDir, 0o555); err != nil {
		t.Fatalf("failed to chmod user log dir: %v", err)
	}
	defer func() {
		_ = os.Chmod(rc.UserLogDir, 0o755)
	}()

	err = rc.Reset(nil, nil)
	if err == nil {
		t.Error("expected error when deleting read-only UserLogDir, got nil")
	}
}

func TestReset_DeleteUserDataDirError(t *testing.T) {
	tempBase := t.TempDir()
	logDir := filepath.Join(tempBase, "logs")
	dataDir := filepath.Join(tempBase, "data")
	appName := "resettest6"

	rc, err := xruntime.Start(appName, runtimeTestFS, logDir, dataDir)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Remove log directory so it succeeds for UserLogDir step
	if rc.CurrentFileSessionLog != nil {
		_ = rc.CurrentFileSessionLog.Close()
		rc.CurrentFileSessionLog = nil
	}
	_ = os.RemoveAll(rc.UserLogDir)
	rc.UserLogDir = ""

	// Make user data directory read-only so deleting files inside fails
	if err := os.Chmod(rc.UserDataDir, 0o555); err != nil {
		t.Fatalf("failed to chmod user data dir: %v", err)
	}
	defer func() {
		_ = os.Chmod(rc.UserDataDir, 0o755)
	}()

	err = rc.Reset(nil, nil)
	if err == nil {
		t.Error("expected error when deleting read-only UserDataDir, got nil")
	}
}
