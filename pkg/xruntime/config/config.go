package config

import (
	"embed"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AeonDigital/Go-Core-xconfig/pkg/xconfig"
	"github.com/AeonDigital/Go-Core-xconfig/pkg/xconfig/parser/yaml"
	"github.com/AeonDigital/Go-Core-xerrors/pkg/xerrors"
	"github.com/AeonDigital/Go-Core-xfs/pkg/xfs"
	"github.com/AeonDigital/Go-Core-xruntime/pkg/xruntime/types"
	"github.com/AeonDigital/Go-Core-xruntime/pkg/xruntime/utils"
)

func InitAppConfiguration(
	appName string,
	appFS embed.FS,
	logDir string,
	dataDir string,
) error {
	types.AppConfig.AppFS = appFS

	err := appConfiguration_Define_LocalAppFileSystem(appName, logDir, dataDir)
	if err != nil {
		return err
	}

	err = appConfiguration_Delete_ConfigJson_If_Outdated()
	if err != nil {
		return err
	}

	err = appConfiguration_Create_LocalFileSystem()
	if err != nil {
		return err
	}

	if !xfs.Exists(types.AppConfig.UserFileConfigJson) {
		err = appConfiguration_Populate_AppConfig_From_ConfigYAML()
		if err != nil {
			return err
		}

		err = appConfiguration_Rewrite_AppConfig_Properties()
		if err != nil {
			return err
		}

		err = appConfiguration_AppConfig_Logging_CheckConfiguration()
		if err != nil {
			return err
		}

		err = appConfiguration_AppConfig_DBConfig_CheckConfiguration()
		if err != nil {
			return err
		}

		err = appConfiguration_Create_ConfigJson_From_AppConfig()
		if err != nil {
			return err
		}
	}

	err = appConfiguration_Populate_AppConfig_From_ConfigJson()
	if err != nil {
		return err
	}

	err = appConfiguration_DBConfig_InitDB()
	if err != nil {
		return err
	}

	err = appConfiguration_Logging_InitLog()
	if err != nil {
		return err
	}

	return nil
}

func EndAppConfiguration() error {
	var err error
	var fileStat os.FileInfo
	storeCurrentLogFile := false

	if !types.AppConfig.Logging.LogRegistry {
		return nil
	}

	if !xfs.Exists(types.AppConfig.UserFileSessionLog) {
		return nil
	}

	err = types.AppConfig.CurrentFileSessionLog.Close()
	if err != nil {
		return err
	}

	fileStat, err = os.Stat(types.AppConfig.UserFileSessionLog)
	if err != nil {
		return err
	}

	if fileStat.Size() > int64(types.AppConfig.Logging.LogRegistryFileMaxSize) {
		storeCurrentLogFile = true
	}
	if !storeCurrentLogFile && time.Since(fileStat.ModTime()) > types.AppConfig.Logging.LogRegistryFileMaxAge.Duration {
		storeCurrentLogFile = true
	}

	if storeCurrentLogFile {
		var useStoreFileName string
		useStoreFileName = strings.ReplaceAll(types.AppConfig.CurrentDateTime, ":", "_")
		useStoreFileName = strings.ReplaceAll(useStoreFileName, "-", "_")
		useStoreFileName = strings.ReplaceAll(useStoreFileName, " ", "-")
		useStoreFileName = useStoreFileName + ".log"

		storedUserFileSessionLog := filepath.Join(types.AppConfig.UserLogDir, useStoreFileName)
		err = xfs.CopyFile(types.AppConfig.UserFileSessionLog, storedUserFileSessionLog)
		if err != nil {
			return err
		}

		err = xfs.DeleteFile(types.AppConfig.UserFileSessionLog)
		if err != nil {
			return err
		}
	}

	return nil
}

func appConfiguration_Define_LocalAppFileSystem(
	appName string,
	logDir string,
	dataDir string,
) error {
	var err error

	if logDir == "" {
		// Directory where logs are save
		logDir, err = xfs.GetUserLogDir()
		if err != nil {
			return err
		}
	}

	if dataDir == "" {
		// Directory where local data files are save
		dataDir, err = xfs.GetUserDataDir()
		if err != nil {
			return err
		}
	}

	// path to current log dir
	types.AppConfig.UserLogDir = filepath.Join(logDir, appName)

	// path to current data dir
	types.AppConfig.UserDataDir = filepath.Join(dataDir, appName)

	// Path to current log file
	types.AppConfig.UserFileSessionLog = filepath.Join(types.AppConfig.UserLogDir, types.FileSessionLog)

	// Path to current config YAML
	types.AppConfig.UserFileConfigYAML = filepath.Join(types.AppConfig.UserDataDir, types.DirAppFSYAML, types.FileConfigYAML)

	// Path to current config json
	types.AppConfig.UserFileConfigJson = filepath.Join(types.AppConfig.UserDataDir, types.FileConfigJson)

	// Path to current sqlite database
	types.AppConfig.UserFileSQLiteData = filepath.Join(types.AppConfig.UserDataDir, types.FileSQLiteData)

	return nil
}

func appConfiguration_Delete_ConfigJson_If_Outdated() error {
	if xfs.Exists(types.AppConfig.UserFileConfigYAML) && xfs.Exists(types.AppConfig.UserFileConfigJson) {
		isNewYAML, _, _, err := utils.IsFileNewer(types.AppConfig.UserFileConfigYAML, types.AppConfig.UserFileConfigJson)
		if err != nil {
			return err
		}

		if isNewYAML {
			err = xfs.DeleteFile(types.AppConfig.UserFileConfigJson)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func appConfiguration_Create_LocalFileSystem() error {
	err := utils.CreateDirIfNotExists(types.AppConfig.UserLogDir)
	if err != nil {
		return err
	}

	err = utils.CreateDirIfNotExists(types.AppConfig.UserDataDir)
	if err != nil {
		return err
	}

	replaceMode := "none"
	createBackup := false

	if !xfs.Exists(types.AppConfig.UserFileConfigYAML) || !xfs.Exists(types.AppConfig.UserFileConfigJson) {
		replaceMode = "all"
		createBackup = true
	}

	return utils.ExtractEmbedFS(
		types.AppConfig.AppFS,
		types.AppConfig.UserDataDir,
		replaceMode,
		createBackup,
	)
}

func appConfiguration_Populate_AppConfig_From_ConfigYAML() error {
	var parsers []xconfig.Parser
	var options []xconfig.Options

	parserYAML := yaml.NewParser()
	optionsYAML := xconfig.Options{
		FilePath: types.AppConfig.UserFileConfigYAML,
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

	err = mgmtConfig.Populate(types.AppConfig)
	if err != nil {
		return err
	}

	return nil
}

func appConfiguration_Rewrite_AppConfig_Properties() error {
	types.AppConfig.DBConfig.MigrationsDirPath = strings.Replace(
		types.AppConfig.DBConfig.MigrationsDirPath,
		"[[USER_DATA_DIR]]",
		types.AppConfig.UserDataDir,
		1,
	)

	return nil
}

func appConfiguration_AppConfig_Logging_CheckConfiguration() error {
	err := types.AppConfig.Logging.CheckConfiguration(types.FileSessionLog)
	if err != nil {
		return err
	}

	types.AppConfig.CurrentDateTime = time.Now().Format(types.AppConfig.Logging.UseTimeFormat)
	return nil
}

func appConfiguration_AppConfig_DBConfig_CheckConfiguration() error {
	if types.AppConfig.DBConfig.SQLite.Dir == "" {
		types.AppConfig.DBConfig.SQLite.Dir = types.AppConfig.UserDataDir
	}
	if types.AppConfig.DBConfig.SQLite.FileName == "" {
		types.AppConfig.DBConfig.SQLite.FileName = types.FileSQLiteData
	}

	return types.AppConfig.DBConfig.CheckConfiguration()
}

func appConfiguration_Create_ConfigJson_From_AppConfig() error {
	appConfigJsonBytes, err := json.Marshal(types.AppConfig)
	if err != nil {
		return err
	}

	return os.WriteFile(types.AppConfig.UserFileConfigJson, appConfigJsonBytes, 0o600)
}

func appConfiguration_Populate_AppConfig_From_ConfigJson() error {
	appConfigJsonBytes, err := os.ReadFile(types.AppConfig.UserFileConfigJson)
	if err != nil {
		return err
	}

	if len(appConfigJsonBytes) == 0 {
		err = xfs.DeleteFile(types.AppConfig.UserFileConfigJson)
		if err != nil {
			return err
		}

		nErr := xerrors.NewErrorCLI().
			SetDevMessage("configuration file '%s' is corrupted or empty", types.AppConfig.UserFileConfigJson).
			SetUserMessage("configuration file '%s' is corrupted or empty", types.AppConfig.UserFileConfigJson)

		return nErr
	}

	err = json.Unmarshal(appConfigJsonBytes, types.AppConfig)
	if err != nil {
		return err
	}

	return nil
}

func appConfiguration_DBConfig_RefreshSQLiteDSN() error {
	sqlite := &types.AppConfig.DBConfig.SQLite

	if sqlite.Dir == "" && sqlite.FileName == "" {
		return nil
	}

	types.AppConfig.DBConfig.DSN = ""
	return types.AppConfig.DBConfig.CheckConfiguration()
}

func appConfiguration_DBConfig_InitDB() error {
	runMigrations := !xfs.Exists(types.AppConfig.UserFileSQLiteData)

	err := appConfiguration_DBConfig_RefreshSQLiteDSN()
	if err != nil {
		return err
	}

	err = types.AppConfig.DBConfig.InitDataBaseConnection(types.AppConfig.Context)
	if err != nil {
		return err
	}

	if runMigrations {
		err = types.AppConfig.DBConfig.RunMigrations(types.AppConfig.Context)
		if err != nil {
			return err
		}
	}

	return nil
}

func appConfiguration_Logging_InitLog() error {
	var err error

	slog.SetDefault(
		slog.New(&types.AppConfig.Logging),
	)

	if !types.AppConfig.Logging.LogRegistry {
		return nil
	}

	if !xfs.Exists(types.AppConfig.UserFileSessionLog) {
		tmpFile, err := xfs.OpenFileWrite(types.AppConfig.UserFileSessionLog, false)
		if err != nil {
			return err
		}

		_, err = tmpFile.WriteString(types.AppConfig.CurrentDateTime + "\n\n")
		if err != nil {
			return err
		}

		err = tmpFile.Close()
		if err != nil {
			return err
		}
	}

	types.AppConfig.CurrentFileSessionLog, err = xfs.OpenFileWrite(types.AppConfig.UserFileSessionLog, false)
	if err != nil {
		return err
	}

	return nil
}
