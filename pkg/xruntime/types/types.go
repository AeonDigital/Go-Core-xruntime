package types

import (
	"context"
	"embed"
	"os"

	"github.com/AeonDigital/Go-Core-Utils/module/xlog/pkg/xlog"
	"github.com/AeonDigital/Go-Core-xdb/pkg/xdb"
)

const (
	DirAppFSYAML string = "appfs/yaml"

	FileSessionLog string = "current.log"
	FileConfigYAML string = "config.yaml"
	FileConfigJson string = "config.json"
	FileSQLiteData string = "sqlite.db"

	CmdOptionAll string = "all"
)

var AppConfig *AppConfiguration = &AppConfiguration{
	Context: context.Background(),
}

type AppConfiguration struct {
	AppName string `json:"appName"`
	Version string `json:"version"`
	Debug   bool   `json:"debug"`

	AppFS embed.FS `json:"-"`

	UserLogDir  string
	UserDataDir string

	UserFileSessionLog string
	UserFileConfigJson string
	UserFileConfigYAML string
	UserFileSQLiteData string

	UserDirSQLiteData string

	CurrentFileSessionLog *os.File
	CurrentDateTime       string

	Logging  xlog.LogHandler `json:"logging"`
	DBConfig xdb.DBConfig    `json:"dbConfig"`

	Context context.Context `json:"-"`
}
