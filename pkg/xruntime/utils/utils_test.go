package utils_test

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AeonDigital/Go-Core-xruntime/pkg/xruntime/utils"
)

//go:embed testfs/*
var testEmbedFS embed.FS

func TestIsFileNewer(t *testing.T) {
	tempDir := t.TempDir()

	fileA := filepath.Join(tempDir, "fileA.txt")
	fileB := filepath.Join(tempDir, "fileB.txt")

	if err := os.WriteFile(fileA, []byte("AAA"), 0o600); err != nil {
		t.Fatalf("failed to write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("BBB"), 0o600); err != nil {
		t.Fatalf("failed to write fileB: %v", err)
	}

	tA := time.Now().Add(-10 * time.Minute)
	tB := time.Now().Add(-20 * time.Minute)

	// Case 1: fileA is newer than fileB
	if err := os.Chtimes(fileA, tA, tA); err != nil {
		t.Fatalf("failed to set times for fileA: %v", err)
	}
	if err := os.Chtimes(fileB, tB, tB); err != nil {
		t.Fatalf("failed to set times for fileB: %v", err)
	}

	isANewer, newer, older, err := utils.IsFileNewer(fileA, fileB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isANewer {
		t.Errorf("expected isANewer to be true")
	}
	if newer != fileA || older != fileB {
		t.Errorf("expected newer=%s older=%s, got newer=%s older=%s", fileA, fileB, newer, older)
	}

	// Case 2: fileB is newer than fileA
	tA2 := time.Now().Add(-30 * time.Minute)
	tB2 := time.Now().Add(-5 * time.Minute)
	if err := os.Chtimes(fileA, tA2, tA2); err != nil {
		t.Fatalf("failed to set times for fileA: %v", err)
	}
	if err := os.Chtimes(fileB, tB2, tB2); err != nil {
		t.Fatalf("failed to set times for fileB: %v", err)
	}

	isANewer, newer, older, err = utils.IsFileNewer(fileA, fileB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isANewer {
		t.Errorf("expected isANewer to be false")
	}
	if newer != fileB || older != fileA {
		t.Errorf("expected newer=%s older=%s, got newer=%s older=%s", fileB, fileA, newer, older)
	}

	// Case 3: fileA does not exist
	_, _, _, err = utils.IsFileNewer(filepath.Join(tempDir, "nonexistent.txt"), fileB)
	if err == nil {
		t.Errorf("expected error when fileA does not exist, got nil")
	}

	// Case 4: fileB does not exist
	_, _, _, err = utils.IsFileNewer(fileA, filepath.Join(tempDir, "nonexistent.txt"))
	if err == nil {
		t.Errorf("expected error when fileB does not exist, got nil")
	}
}

func TestCreateDirIfNotExists(t *testing.T) {
	tempDir := t.TempDir()

	targetDir := filepath.Join(tempDir, "nested", "level2", "dir")

	// Case 1: creates non-existing nested directory
	if err := utils.CreateDirIfNotExists(targetDir); err != nil {
		t.Fatalf("expected nil error on creation, got: %v", err)
	}

	info, err := os.Stat(targetDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected targetDir to exist as directory, got err: %v", err)
	}

	// Case 2: idempotent call on already existing directory
	if err := utils.CreateDirIfNotExists(targetDir); err != nil {
		t.Fatalf("expected nil error on existing directory, got: %v", err)
	}
}

func TestFindFilesInEmbedFS(t *testing.T) {
	// Case 1: find by glob pattern with wildcards spanning directory hierarchy
	matches, err := utils.FindFilesInEmbedFS(testEmbedFS, "*/*/*.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 || !strings.HasSuffix(matches[0], "config.yaml") {
		t.Errorf("expected to find config.yaml with glob, got: %v", matches)
	}

	// Case 2: find by substring / contains
	matches, err = utils.FindFilesInEmbedFS(testEmbedFS, "sample")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 || !strings.HasSuffix(matches[0], "sample.txt") {
		t.Errorf("expected to find sample.txt, got: %v", matches)
	}

	// Case 3: case-insensitivity
	matches, err = utils.FindFilesInEmbedFS(testEmbedFS, "CONFIG.YAML")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("expected case-insensitive match for CONFIG.YAML, got: %v", matches)
	}

	// Case 4: pattern that matches nothing
	matches, err = utils.FindFilesInEmbedFS(testEmbedFS, "nonexistent_pattern_*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected empty slice for unmatched pattern, got: %v", matches)
	}

	// Case 5: empty embedFS
	var emptyFS embed.FS
	matches, err = utils.FindFilesInEmbedFS(emptyFS, "*")
	if err != nil {
		t.Fatalf("unexpected error with empty embedFS: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty embedFS, got: %v", matches)
	}
}

func TestExtractEmbedFS_EmptyFS(t *testing.T) {
	tempDir := t.TempDir()
	var emptyFS embed.FS

	if err := utils.ExtractEmbedFS(emptyFS, tempDir, "none", false); err != nil {
		t.Fatalf("expected nil error on empty embed.FS, got: %v", err)
	}
}

func TestExtractEmbedFS_ReplaceModeNone(t *testing.T) {
	tempDir := t.TempDir()

	// Initial extraction: target does not exist, should extract everything
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "none", false); err != nil {
		t.Fatalf("initial extraction failed: %v", err)
	}

	configPath := filepath.Join(tempDir, "testfs", "appfs", "config.yaml")
	samplePath := filepath.Join(tempDir, "testfs", "appfs", "nested", "sample.txt")

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected %s to exist after extraction: %v", configPath, err)
	}
	if _, err := os.Stat(samplePath); err != nil {
		t.Fatalf("expected %s to exist after extraction: %v", samplePath, err)
	}

	// Modify a local file
	if err := os.WriteFile(configPath, []byte("modified: true"), 0o600); err != nil {
		t.Fatalf("failed to modify configPath: %v", err)
	}

	// Run replaceMode="none" again when directory already exists: should abort and NOT overwrite
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "none", false); err != nil {
		t.Fatalf("second extraction with none failed: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read configPath: %v", err)
	}
	if string(content) != "modified: true" {
		t.Errorf("expected local file to remain unchanged under replaceMode=none, got: %s", string(content))
	}
}

func TestExtractEmbedFS_ReplaceModeAllAndBackup(t *testing.T) {
	tempDir := t.TempDir()

	// Initial extraction
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "all", false); err != nil {
		t.Fatalf("initial extraction failed: %v", err)
	}

	configPath := filepath.Join(tempDir, "testfs", "appfs", "config.yaml")
	if err := os.WriteFile(configPath, []byte("locally modified content"), 0o600); err != nil {
		t.Fatalf("failed to modify config: %v", err)
	}

	// Run with replaceMode="all" and backup=true
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "all", true); err != nil {
		t.Fatalf("extraction with all and backup failed: %v", err)
	}

	// Verify local file was overwritten back to embedded content
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if strings.Contains(string(content), "locally modified content") {
		t.Errorf("expected file to be restored from embedded FS under replaceMode=all")
	}

	// Verify backup directory exists
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read tempDir: %v", err)
	}

	hasBackup := false
	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), "-bk-") {
			hasBackup = true
			break
		}
	}
	if !hasBackup {
		t.Errorf("expected a backup directory with '-bk-' in name, found entries: %v", entries)
	}
}

func TestExtractEmbedFS_ReplaceModeMiss(t *testing.T) {
	tempDir := t.TempDir()

	// Initial extraction
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "all", false); err != nil {
		t.Fatalf("initial extraction failed: %v", err)
	}

	configPath := filepath.Join(tempDir, "testfs", "appfs", "config.yaml")
	samplePath := filepath.Join(tempDir, "testfs", "appfs", "nested", "sample.txt")

	// Modify configPath and delete samplePath
	if err := os.WriteFile(configPath, []byte("user custom setting"), 0o600); err != nil {
		t.Fatalf("failed to modify configPath: %v", err)
	}
	if err := os.Remove(samplePath); err != nil {
		t.Fatalf("failed to remove samplePath: %v", err)
	}

	// Run replaceMode="miss"
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "miss", false); err != nil {
		t.Fatalf("extraction with miss failed: %v", err)
	}

	// Verify existing file was preserved
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read configPath: %v", err)
	}
	if string(content) != "user custom setting" {
		t.Errorf("expected modified file to be untouched in 'miss' mode, got: %s", string(content))
	}

	// Verify missing file was recreated
	if _, err := os.Stat(samplePath); err != nil {
		t.Errorf("expected missing file to be recreated in 'miss' mode: %v", err)
	}
}

func TestExtractEmbedFS_ReplaceModeChanged(t *testing.T) {
	tempDir := t.TempDir()

	// Initial extraction
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "all", false); err != nil {
		t.Fatalf("initial extraction failed: %v", err)
	}

	configPath := filepath.Join(tempDir, "testfs", "appfs", "config.yaml")

	// Modify configPath so hashes diverge
	if err := os.WriteFile(configPath, []byte("tampered content"), 0o600); err != nil {
		t.Fatalf("failed to alter file: %v", err)
	}

	// Run replaceMode="changed"
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "changed", false); err != nil {
		t.Fatalf("extraction with changed failed: %v", err)
	}

	// Divergent file should be overwritten
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read configPath: %v", err)
	}
	if string(content) == "tampered content" {
		t.Errorf("expected divergent file to be overwritten under 'changed' mode")
	}

	// Run replaceMode="changed" again where hashes are now identical
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "changed", false); err != nil {
		t.Fatalf("second extraction with changed failed: %v", err)
	}
}

func TestExtractEmbedFS_BackupWhenNotExists(t *testing.T) {
	tempDir := t.TempDir()

	// Target does not exist, backup=true should not fail even though local root does not exist yet
	if err := utils.ExtractEmbedFS(testEmbedFS, tempDir, "all", true); err != nil {
		t.Fatalf("expected nil error when backup=true on non-existing target, got: %v", err)
	}

	configPath := filepath.Join(tempDir, "testfs", "appfs", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("expected extracted file to exist: %v", err)
	}
}
