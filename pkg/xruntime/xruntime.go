// Package xruntime provides application runtime bootstrapping, configuration loading,
// directory provisioning, embedded asset extraction, logging, and database initialization.
package xruntime

import (
	"context"
	"embed"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AeonDigital/Go-Core-xconfig/pkg/xconfig"
	"github.com/AeonDigital/Go-Core-xconfig/pkg/xconfig/parser/yaml"
	"github.com/AeonDigital/Go-Core-xdb/pkg/xdb"
	"github.com/AeonDigital/Go-Core-xerrors/pkg/xerrors"
	"github.com/AeonDigital/Go-Core-xfs/pkg/xfs"
	"github.com/AeonDigital/Go-Core-xruntime/pkg/xruntime/utils"
	"github.com/AeonDigital/Go-Core-xutils/module/xlog/pkg/xlog"
)

const (
	// DirAppFSYAML is the default relative subfolder inside the user data directory where YAML config files reside.
	DirAppFSYAML string = "appfs/yaml"

	// FileSessionLog is the file name used for the active runtime session log.
	FileSessionLog string = "current.log"

	// FileConfigYAML is the primary YAML configuration file name.
	FileConfigYAML string = "config.yaml"

	// FileConfigJson is the cached JSON configuration file name generated from the parsed YAML.
	FileConfigJson string = "config.json"

	// FileSQLiteData is the default SQLite database file name.
	FileSQLiteData string = "sqlite.db"

	// CmdOptionAll is a constant indicating a blanket selection or replacement mode.
	CmdOptionAll string = "all"
)

// NewRuntimeConfig creates a new uninitialized RuntimeConfig instance with a default background context.
//
// Returns:
//   - *RuntimeConfig: A new instance of RuntimeConfig.
func NewRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		Context: context.Background(),
	}
}

// RuntimeConfig holds the runtime state, paths, configuration metadata, logger, and database connection for an application instance.
type RuntimeConfig struct {
	// AppName is the unique identifier name of the application.
	AppName string `json:"appName"`

	// Version is the semantic version string of the application.
	Version string `json:"version"`

	// Debug indicates whether debug mode is enabled.
	Debug bool `json:"debug"`

	// AppFS is the embedded filesystem holding default configurations, schemas, and assets.
	AppFS embed.FS `json:"-"`

	// UserLogDir is the directory path where application log files are stored.
	UserLogDir string

	// UserDataDir is the directory path where application data and configuration files are stored.
	UserDataDir string

	// UserFileSessionLog is the full file path to the active session log.
	UserFileSessionLog string

	// UserFileConfigJson is the full file path to the compiled JSON configuration cache.
	UserFileConfigJson string

	// UserFileConfigYAML is the full file path to the source YAML configuration file.
	UserFileConfigYAML string

	// UserFileSQLiteData is the full file path to the SQLite database file.
	UserFileSQLiteData string

	// UserDirSQLiteData is the directory containing the SQLite database.
	UserDirSQLiteData string

	// CurrentFileSessionLog is the open file descriptor for writing current session logs.
	CurrentFileSessionLog *os.File

	// CurrentDateTime is the timestamp string representing when the session was initiated.
	CurrentDateTime string

	// Logging contains structured logging configurations and handlers.
	Logging xlog.LogHandler `json:"logging"`

	// DBConfig contains database connection parameters and migration settings.
	DBConfig xdb.DBConfig `json:"dbConfig"`

	// Context is the root context governing the lifecycle of the runtime instance.
	Context context.Context `json:"-"`
}

// PreResetFunc represents a callback function executed prior to deleting application runtime directories.
// It returns a boolean flag indicating if reset should proceed and an error if pre-validation fails.
type PreResetFunc func() (bool, error)

// PostResetFunc represents a callback function executed after application runtime directories are deleted.
// It returns an error if post-reset cleanup operations fail.
type PostResetFunc func() error

// End gracefully finalizes the runtime configuration, closing active log files and performing log rotations if needed.
//
// Returns:
//   - error: An error if closing log files or archiving logs fails, nil otherwise.
func (rc *RuntimeConfig) End() error {
	return End(rc)
}

// Reset deletes the application log and user data directories associated with the runtime configuration.
//
// Parameters:
//   - preReset: Optional callback executed before resetting directories.
//   - posReset: Optional callback executed after resetting directories.
//
// Returns:
//   - error: An error if preReset fails or returns false, directory deletion fails, or posReset fails; nil otherwise.
func (rc *RuntimeConfig) Reset(preReset PreResetFunc, posReset PostResetFunc) error {
	return Reset(rc, preReset, posReset)
}

// Start initializes the runtime filesystem, loads configuration, connects to the database, and starts logging.
//
// Parameters:
//   - appName: Unique name of the application used for directory partitioning.
//   - appFS: Embedded filesystem containing assets and initial configuration files.
//   - logDir: Optional custom directory path for logs (uses OS user log directory if empty).
//   - dataDir: Optional custom directory path for application data (uses OS user data directory if empty).
//
// Returns:
//   - *RuntimeConfig: Fully initialized runtime configuration instance ready for operation.
//   - error: An error if any initialization phase (filesystem, config parsing, DB connection, logging) fails.
func Start(
	appName string,
	appFS embed.FS,
	logDir string,
	dataDir string,
) (*RuntimeConfig, error) {
	runtimeConfig := NewRuntimeConfig()
	runtimeConfig.AppFS = appFS

	err := runtime_Define_LocalAppFileSystem(runtimeConfig, appName, logDir, dataDir)
	if err != nil {
		return nil, err
	}

	err = runtime_Delete_ConfigJson_If_Outdated(runtimeConfig)
	if err != nil {
		return nil, err
	}

	err = runtime_Create_LocalFileSystem(runtimeConfig)
	if err != nil {
		return nil, err
	}

	if !xfs.Exists(runtimeConfig.UserFileConfigJson) {
		err = runtime_Populate_From_ConfigYAML(runtimeConfig)
		if err != nil {
			return nil, err
		}

		runtimeConfig.AppName = appName

		err = runtime_Rewrite_Properties(runtimeConfig)
		if err != nil {
			return nil, err
		}

		err = runtime_Logging_CheckConfiguration(runtimeConfig)
		if err != nil {
			return nil, err
		}

		err = runtime_DBConfig_CheckConfiguration(runtimeConfig)
		if err != nil {
			return nil, err
		}

		err = runtime_Create_ConfigJson(runtimeConfig)
		if err != nil {
			return nil, err
		}
	} else {
		err = runtime_Populate_From_ConfigJson(runtimeConfig)
		if err != nil {
			return nil, err
		}

		runtime_Update_LocalAppFileSystem(runtimeConfig, appName, logDir, dataDir)

		err = runtime_Logging_CheckConfiguration(runtimeConfig)
		if err != nil {
			return nil, err
		}

		err = runtime_Create_ConfigJson(runtimeConfig)
		if err != nil {
			return nil, err
		}
	}

	err = runtime_DBConfig_InitDB(runtimeConfig)
	if err != nil {
		return nil, err
	}

	err = runtime_Logging_InitLog(runtimeConfig)
	if err != nil {
		return nil, err
	}

	return runtimeConfig, nil
}

// End gracefully finalizes the given runtime configuration instance.
// It closes active log file handles and rotates the session log if size or duration limits are exceeded.
//
// Parameters:
//   - runtimeConfig: The active runtime configuration instance to close.
//
// Returns:
//   - error: An error if closing log files, rotating, or cleaning up fails, nil otherwise.
func End(runtimeConfig *RuntimeConfig) error {
	if runtimeConfig == nil {
		return nil
	}

	var err error
	var fileStat os.FileInfo
	storeCurrentLogFile := false

	if !runtimeConfig.Logging.LogRegistry {
		return nil
	}

	if !xfs.Exists(runtimeConfig.UserFileSessionLog) {
		return nil
	}

	if runtimeConfig.CurrentFileSessionLog != nil {
		err = runtimeConfig.CurrentFileSessionLog.Close()
		if err != nil {
			return err
		}
	}

	fileStat, err = os.Stat(runtimeConfig.UserFileSessionLog)
	if err != nil {
		return err
	}

	if fileStat.Size() > int64(runtimeConfig.Logging.LogRegistryFileMaxSize) {
		storeCurrentLogFile = true
	}
	if !storeCurrentLogFile && time.Since(fileStat.ModTime()) > runtimeConfig.Logging.LogRegistryFileMaxAge.Duration {
		storeCurrentLogFile = true
	}

	if storeCurrentLogFile {
		var useStoreFileName string
		useStoreFileName = strings.ReplaceAll(runtimeConfig.CurrentDateTime, ":", "_")
		useStoreFileName = strings.ReplaceAll(useStoreFileName, "-", "_")
		useStoreFileName = strings.ReplaceAll(useStoreFileName, " ", "-")
		useStoreFileName = useStoreFileName + ".log"

		storedUserFileSessionLog := filepath.Join(runtimeConfig.UserLogDir, useStoreFileName)
		err = xfs.CopyFile(runtimeConfig.UserFileSessionLog, storedUserFileSessionLog)
		if err != nil {
			return err
		}

		err = xfs.DeleteFile(runtimeConfig.UserFileSessionLog)
		if err != nil {
			return err
		}
	}

	return nil
}

// Reset removes the application log and user data directories associated with the given runtime configuration.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//   - preReset: Optional function executed before deleting directories. If provided, reset proceeds only if it returns (true, nil).
//   - posReset: Optional function executed after deleting directories.
//
// Returns:
//   - error: An error if preReset returns an error or false, directory deletion fails, or posReset fails; nil otherwise.
func Reset(
	runtimeConfig *RuntimeConfig,
	preReset PreResetFunc,
	posReset PostResetFunc,
) error {
	if runtimeConfig == nil {
		return nil
	}

	if preReset != nil {
		ok, err := preReset()
		if err != nil {
			return err
		}
		if !ok {
			return xerrors.NewErrorCLI().
				SetDevMessage("could not perform reset due to primary conditions failure").
				SetUserMessage("could not perform reset due to primary conditions failure")
		}
	}

	if runtimeConfig.CurrentFileSessionLog != nil {
		_ = runtimeConfig.CurrentFileSessionLog.Close()
		runtimeConfig.CurrentFileSessionLog = nil
	}

	if runtimeConfig.DBConfig.DB != nil {
		_ = runtimeConfig.DBConfig.DB.Close()
		runtimeConfig.DBConfig.DB = nil
	}

	var err error
	if runtimeConfig.UserLogDir != "" && xfs.Exists(runtimeConfig.UserLogDir) {
		err = xfs.DeleteDir(runtimeConfig.UserLogDir, true)
		if err != nil {
			return err
		}
	}

	if runtimeConfig.UserDataDir != "" && xfs.Exists(runtimeConfig.UserDataDir) {
		err = xfs.DeleteDir(runtimeConfig.UserDataDir, true)
		if err != nil {
			return err
		}
	}

	if posReset != nil {
		err = posReset()
		if err != nil {
			return err
		}
	}

	return nil
}

// runtime_Define_LocalAppFileSystem sets up and resolves application directory and file paths on the runtime configuration.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//   - appName: Application name for subdirectory namespacing.
//   - logDir: Custom log directory path or empty string to use the system default.
//   - dataDir: Custom data directory path or empty string to use the system default.
//
// Returns:
//   - error: An error if determining system directories fails; nil on success.
func runtime_Define_LocalAppFileSystem(
	runtimeConfig *RuntimeConfig,
	appName string,
	logDir string,
	dataDir string,
) error {
	var err error

	if logDir == "" {
		// Directory where logs are saved
		logDir, err = xfs.GetUserLogDir()
		if err != nil {
			return err
		}
	}

	if dataDir == "" {
		// Directory where local data files are saved
		dataDir, err = xfs.GetUserDataDir()
		if err != nil {
			return err
		}
	}

	runtimeConfig.AppName = appName

	// path to current log dir
	runtimeConfig.UserLogDir = filepath.Join(logDir, appName)

	// path to current data dir
	runtimeConfig.UserDataDir = filepath.Join(dataDir, appName)

	// Path to current log file
	runtimeConfig.UserFileSessionLog = filepath.Join(runtimeConfig.UserLogDir, FileSessionLog)

	// Path to current config YAML
	runtimeConfig.UserFileConfigYAML = filepath.Join(runtimeConfig.UserDataDir, DirAppFSYAML, FileConfigYAML)

	// Path to current config json
	runtimeConfig.UserFileConfigJson = filepath.Join(runtimeConfig.UserDataDir, FileConfigJson)

	// Path to current sqlite database
	runtimeConfig.UserFileSQLiteData = filepath.Join(runtimeConfig.UserDataDir, FileSQLiteData)

	return nil
}

// runtime_Update_LocalAppFileSystem overrides application directory and file paths when custom log or data directories are provided.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//   - appName: Application name for subdirectory namespacing.
//   - logDir: Custom log directory path or empty string.
//   - dataDir: Custom data directory path or empty string.
func runtime_Update_LocalAppFileSystem(
	runtimeConfig *RuntimeConfig,
	appName string,
	logDir string,
	dataDir string,
) {
	if logDir != "" {
		runtimeConfig.UserLogDir = filepath.Join(logDir, appName)
		runtimeConfig.UserFileSessionLog = filepath.Join(runtimeConfig.UserLogDir, FileSessionLog)
	}
	if dataDir != "" {
		runtimeConfig.UserDataDir = filepath.Join(dataDir, appName)
		runtimeConfig.UserFileConfigYAML = filepath.Join(runtimeConfig.UserDataDir, DirAppFSYAML, FileConfigYAML)
		runtimeConfig.UserFileConfigJson = filepath.Join(runtimeConfig.UserDataDir, FileConfigJson)
		runtimeConfig.UserFileSQLiteData = filepath.Join(runtimeConfig.UserDataDir, FileSQLiteData)
	}
}

// runtime_Delete_ConfigJson_If_Outdated removes the compiled JSON cache if the source YAML config has a newer timestamp.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if checking file timestamps or deleting the JSON cache fails; nil on success.
func runtime_Delete_ConfigJson_If_Outdated(runtimeConfig *RuntimeConfig) error {
	if xfs.Exists(runtimeConfig.UserFileConfigYAML) && xfs.Exists(runtimeConfig.UserFileConfigJson) {
		isNewYAML, _, _, err := utils.IsFileNewer(runtimeConfig.UserFileConfigYAML, runtimeConfig.UserFileConfigJson)
		if err != nil {
			return err
		}

		if isNewYAML {
			err = xfs.DeleteFile(runtimeConfig.UserFileConfigJson)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// runtime_Create_LocalFileSystem ensures local directory trees exist and extracts initial files from AppFS if needed.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if creating directories or extracting embedded filesystem contents fails; nil on success.
func runtime_Create_LocalFileSystem(runtimeConfig *RuntimeConfig) error {
	err := utils.CreateDirIfNotExists(runtimeConfig.UserLogDir)
	if err != nil {
		return err
	}

	err = utils.CreateDirIfNotExists(runtimeConfig.UserDataDir)
	if err != nil {
		return err
	}

	replaceMode := "none"
	createBackup := false

	if !xfs.Exists(runtimeConfig.UserFileConfigYAML) || !xfs.Exists(runtimeConfig.UserFileConfigJson) {
		replaceMode = "all"
		createBackup = true
	}

	return utils.ExtractEmbedFS(
		runtimeConfig.AppFS,
		runtimeConfig.UserDataDir,
		replaceMode,
		createBackup,
	)
}

// runtime_Populate_From_ConfigYAML loads and parses the source YAML configuration file into the RuntimeConfig structure.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance to populate.
//
// Returns:
//   - error: An error if YAML file parsing or population fails; nil on success.
func runtime_Populate_From_ConfigYAML(runtimeConfig *RuntimeConfig) error {
	var parsers []xconfig.Parser
	var options []xconfig.Options

	parserYAML := yaml.NewParser()
	optionsYAML := xconfig.Options{
		FilePath: runtimeConfig.UserFileConfigYAML,
	}
	parsers = append(parsers, parserYAML)
	options = append(options, optionsYAML)

	var mgmtConfig *xconfig.Config
	mgmtConfig, err := xconfig.InitAppConfig(
		parsers,
		options,
	)
	if err != nil {
		return err
	}

	err = mgmtConfig.Populate(runtimeConfig)
	if err != nil {
		return err
	}

	return nil
}

// runtime_Rewrite_Properties replaces variable placeholders (such as [[USER_DATA_DIR]]) within configuration values.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: Always returns nil; placeholder substitutions are performed in place.
func runtime_Rewrite_Properties(runtimeConfig *RuntimeConfig) error {
	runtimeConfig.DBConfig.MigrationsDirPath = strings.Replace(
		runtimeConfig.DBConfig.MigrationsDirPath,
		"[[USER_DATA_DIR]]",
		runtimeConfig.UserDataDir,
		1,
	)

	return nil
}

// runtime_Logging_CheckConfiguration validates logging settings and records the current timestamp formatted per logging rules.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if the logging configuration validation fails; nil on success.
func runtime_Logging_CheckConfiguration(runtimeConfig *RuntimeConfig) error {
	if runtimeConfig.Logging.LogRegistryDirPath == "" || runtimeConfig.Logging.LogRegistryDirPath != runtimeConfig.UserLogDir {
		runtimeConfig.Logging.LogRegistryDirPath = runtimeConfig.UserLogDir
	}

	err := runtimeConfig.Logging.CheckConfiguration(FileSessionLog)
	if err != nil {
		return err
	}

	runtimeConfig.CurrentDateTime = time.Now().Format(runtimeConfig.Logging.UseTimeFormat)
	return nil
}

// runtime_DBConfig_CheckConfiguration verifies database configurations, supplying default directory and file names for SQLite.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if database configuration validation fails; nil on success.
func runtime_DBConfig_CheckConfiguration(runtimeConfig *RuntimeConfig) error {
	if runtimeConfig.DBConfig.SQLite.Dir == "" {
		runtimeConfig.DBConfig.SQLite.Dir = runtimeConfig.UserDataDir
	}
	if runtimeConfig.DBConfig.SQLite.FileName == "" {
		runtimeConfig.DBConfig.SQLite.FileName = FileSQLiteData
	}

	return runtimeConfig.DBConfig.CheckConfiguration()
}

// runtime_Create_ConfigJson serializes and writes the resolved RuntimeConfig structure to the JSON cache file.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if JSON serialization or writing to disk fails; nil on success.
func runtime_Create_ConfigJson(runtimeConfig *RuntimeConfig) error {
	appConfigJsonBytes, err := json.Marshal(runtimeConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(runtimeConfig.UserFileConfigJson, appConfigJsonBytes, 0o600)
}

// runtime_Populate_From_ConfigJson reads the JSON configuration cache and populates the RuntimeConfig structure.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance to populate.
//
// Returns:
//   - error: An error if reading, deleting corrupted files, or unmarshaling fails; nil on success.
func runtime_Populate_From_ConfigJson(runtimeConfig *RuntimeConfig) error {
	appConfigJsonBytes, err := os.ReadFile(runtimeConfig.UserFileConfigJson)
	if err != nil {
		return err
	}

	if len(appConfigJsonBytes) == 0 {
		err = xfs.DeleteFile(runtimeConfig.UserFileConfigJson)
		if err != nil {
			return err
		}

		nErr := xerrors.NewErrorCLI().
			SetDevMessage("configuration file '%s' is corrupted or empty", runtimeConfig.UserFileConfigJson).
			SetUserMessage("configuration file '%s' is corrupted or empty", runtimeConfig.UserFileConfigJson)

		return nErr
	}

	err = json.Unmarshal(appConfigJsonBytes, runtimeConfig)
	if err != nil {
		return err
	}

	return nil
}

// runtime_DBConfig_RefreshSQLiteDSN recalculates and updates the database DSN if SQLite parameters are configured.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if refreshing DSN or revalidating configuration fails; nil on success.
func runtime_DBConfig_RefreshSQLiteDSN(runtimeConfig *RuntimeConfig) error {
	sqlite := &runtimeConfig.DBConfig.SQLite

	if sqlite.Dir == "" && sqlite.FileName == "" {
		return nil
	}

	runtimeConfig.DBConfig.DSN = ""
	return runtimeConfig.DBConfig.CheckConfiguration()
}

// runtime_DBConfig_InitDB opens the database connection and runs pending migrations if the database file is new.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if database connection initialization or migration execution fails; nil on success.
func runtime_DBConfig_InitDB(runtimeConfig *RuntimeConfig) error {
	runMigrations := !xfs.Exists(runtimeConfig.UserFileSQLiteData)

	err := runtime_DBConfig_RefreshSQLiteDSN(runtimeConfig)
	if err != nil {
		return err
	}

	err = runtimeConfig.DBConfig.InitDataBaseConnection(runtimeConfig.Context)
	if err != nil {
		return err
	}

	if runMigrations {
		err = runtimeConfig.DBConfig.RunMigrations(runtimeConfig.Context)
		if err != nil {
			return err
		}
	}

	return nil
}

// runtime_Logging_InitLog sets up the default global slog logger and opens the runtime session log file for writing.
//
// Parameters:
//   - runtimeConfig: Target runtime configuration instance.
//
// Returns:
//   - error: An error if opening or creating the log file fails; nil on success.
func runtime_Logging_InitLog(runtimeConfig *RuntimeConfig) error {
	var err error

	slog.SetDefault(
		slog.New(&runtimeConfig.Logging),
	)

	if !runtimeConfig.Logging.LogRegistry {
		return nil
	}

	if !xfs.Exists(runtimeConfig.UserFileSessionLog) {
		tmpFile, err := xfs.OpenFileWrite(runtimeConfig.UserFileSessionLog, false)
		if err != nil {
			return err
		}

		_, err = tmpFile.WriteString(runtimeConfig.CurrentDateTime + "\n\n")
		if err != nil {
			return err
		}

		err = tmpFile.Close()
		if err != nil {
			return err
		}
	}

	runtimeConfig.CurrentFileSessionLog, err = xfs.OpenFileWrite(runtimeConfig.UserFileSessionLog, false)
	if err != nil {
		return err
	}

	return nil
}
