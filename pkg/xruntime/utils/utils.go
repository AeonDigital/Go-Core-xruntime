package utils

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AeonDigital/Go-Core-xfs/pkg/xfs"
)

// IsFileNewer compares the modification timestamps of two local files.
//
// Parameters:
//   - pathFileA: Absolute or relative file path of the first file.
//   - pathFileB: Absolute or relative file path of the second file.
//
// Returns:
//   - isANewer: True if pathFileA was modified more recently than pathFileB, false otherwise.
//   - newerPath: The path of whichever file was modified more recently.
//   - olderPath: The path of whichever file was modified earlier.
//   - err: An error if os.Stat fails for either file.
func IsFileNewer(
	pathFileA string,
	pathFileB string,
) (
	isANewer bool,
	newerPath string,
	olderPath string,
	err error,
) {
	statA, err := os.Stat(pathFileA)
	if err != nil {
		return false, "", "", err
	}

	statB, err := os.Stat(pathFileB)
	if err != nil {
		return false, "", "", err
	}

	timeFileA := statA.ModTime()
	timeFileB := statB.ModTime()

	if timeFileA.After(timeFileB) {
		return true, pathFileA, pathFileB, nil
	}

	return false, pathFileB, pathFileA, nil
}

// CreateDirIfNotExists checks if a directory path exists and creates all necessary parent directories if it does not.
//
// Parameters:
//   - dirPath: The path of the directory to be verified and created.
//
// Returns:
//   - error: An error if directory creation fails, or nil if successful/already existing.
func CreateDirIfNotExists(
	dirPath string,
) error {
	if !xfs.Exists(dirPath) {
		return xfs.CreateDirPath(dirPath)
	}
	return nil
}

// FindFilesInEmbedFS traverses an embedded filesystem and returns paths of files matching a glob or substring pattern.
//
// Parameters:
//   - embedFS: The embedded filesystem instance to search.
//   - pattern: Glob pattern or case-insensitive substring to search for.
//
// Returns:
//   - matches: A slice of matching file paths relative to the embedded filesystem root.
//   - err: An error if filesystem traversal encounters an issue.
func FindFilesInEmbedFS(
	embedFS embed.FS,
	pattern string,
) ([]string, error) {
	var matches []string
	pattern = strings.ToLower(filepath.Clean(pattern))

	err := fs.WalkDir(embedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		lowerPath := strings.ToLower(path)

		matched, _ := filepath.Match(pattern, lowerPath)
		if matched {
			matches = append(matches, path)
			return nil
		}

		if strings.Contains(lowerPath, pattern) {
			matches = append(matches, path)
			return nil
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return matches, nil
}

// ExtractEmbedFS extracts the directory structure and files from an embed.FS into a target directory.
//
// Supported replaceMode strategies:
//   - "none": If the local root target directory already exists, aborts extraction immediately without modifying files.
//   - "all": Completely removes existing target directory contents and recreates all embedded files from scratch.
//   - "miss": Only writes missing files and directories; keeps existing files intact without overwriting.
//   - "changed": Compares SHA-256 hashes of local files against embedded ones and overwrites only modified files.
//
// Parameters:
//   - embedFS: The source embedded filesystem containing files to extract.
//   - targetDirPath: Local destination directory where files will be written.
//   - replaceMode: Strategy ("none", "all", "miss", "changed") determining overwrite behavior.
//   - backup: When true, creates a timestamped backup copy of existing target directory before replacement.
//
// Returns:
//   - error: An error if directory scanning, backup creation, or file writing fails; nil on success.
func ExtractEmbedFS(
	embedFS embed.FS,
	targetDirPath string,
	replaceMode string,
	backup bool,
) error {
	// 1. Identify the root directory name inside the embed.FS
	var rootDir string
	err := fs.WalkDir(embedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != "." && rootDir == "" {
			// Captures the first top-level entry (e.g., "appfs")
			rootDir = strings.Split(path, "/")[0]
			return fs.SkipDir // Stop scanning early once root is found
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan embedded filesystem layout: %w", err)
	}

	// If the embedded FS is completely empty, there is nothing to extract
	if rootDir == "" {
		return nil
	}

	localRootPath := filepath.Join(targetDirPath, rootDir)
	localRootExists := false
	if _, err := os.Stat(localRootPath); err == nil {
		localRootExists = true
	}

	// 2. Evaluate "none" mode immediately before triggering backups or alterations
	if replaceMode == "none" && localRootExists {
		return nil
	}

	// 3. Perform a snapshot backup if requested and the local target directory exists
	if backup && localRootExists {
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		backupPath := fmt.Sprintf("%s-bk-%s", localRootPath, timestamp)

		if err := copyLocalDir(localRootPath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup snapshot: %w", err)
		}
	}

	// 4. Handle the destructive "all" mode clean-up phase
	if replaceMode == "all" && localRootExists {
		if err := os.RemoveAll(localRootPath); err != nil {
			return fmt.Errorf("failed to purge existing local root directory: %w", err)
		}
	}

	// 5. Traverse and recreate the embedded structure on disk
	return fs.WalkDir(embedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}

		targetPath := filepath.Join(targetDirPath, path)

		// Recreate Directory Structure
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		// Recreate Files based on selected replaceMode strategies
		if _, err := os.Stat(targetPath); err == nil {
			// File already exists locally
			switch replaceMode {
			case "miss":
				// Skip file extraction entirely
				return nil

			case "changed":
				// Read embedded file content
				embedContent, err := embedFS.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read embedded file %s: %w", path, err)
				}

				// Compute hashes to test content convergence
				embedHash := getSHA256(embedContent)
				localHash, err := getFileSHA256(targetPath)
				if err != nil {
					return fmt.Errorf("failed to hash local file %s: %w", targetPath, err)
				}

				// If files converge, preserve the local version
				if embedHash == localHash {
					return nil
				}
				// If hashes diverge, execution falls through to overwrite the file
			}
		}

		// Read and write file payload to target location
		content, err := embedFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded source file %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to enforce parent directory structure for %s: %w", targetPath, err)
		}

		if err := os.WriteFile(targetPath, content, 0o644); err != nil {
			return fmt.Errorf("failed to write local file instance %s: %w", targetPath, err)
		}

		return nil
	})
}

// getSHA256 computes the SHA-256 hash string for an in-memory byte slice.
//
// Parameters:
//   - data: Byte slice to be hashed.
//
// Returns:
//   - string: Hexadecimal encoded SHA-256 hash string.
func getSHA256(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// getFileSHA256 computes the SHA-256 hash of a local file on disk.
//
// Parameters:
//   - filePath: Path to the file to be read and hashed.
//
// Returns:
//   - string: Hexadecimal encoded SHA-256 hash string.
//   - error: Error if the file cannot be opened or read.
func getFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// copyLocalDir recursively copies all directories and files from src to dst.
//
// Parameters:
//   - src: Source directory path.
//   - dst: Destination directory path.
//
// Returns:
//   - error: Error if reading, creating, or copying any file/directory fails; nil on success.
func copyLocalDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Perform traditional streaming file copy operation
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
		return nil
	})
}
