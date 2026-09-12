Go-Core-xruntime
================================

![Go Test Coverage](https://raw.githubusercontent.com/github.com/AeonDigital/Go-Core-xruntime/badges/.badges/main/coverage.svg)

> [Aeon Digital](http://www.aeondigital.com.br)  
> rianna@aeondigital.com.br

&nbsp;

> xruntime is a foundation for composing and operating the lifecycle of Go applications.

`xruntime` is an infrastructure library for Go applications, focused on CLIs and standalone tools that require a consistent lifecycle. It centralizes application directory creation, embedded asset provisioning, configuration loading and persistence, initialization of logging, databases, and migrations, and graceful resource shutdown.




&nbsp;
________________________________________________________________________________

## 1. INSTALLATION

Install the package into your Go module using `go get`:

```shell
go get github.com/AeonDigital/Go-Core-xruntime@latest
```




&nbsp;
________________________________________________________________________________

## 2. ARCHITECTURE & LIFECYCLE

`xruntime` eliminates repetitive bootstrap boilerplate by coordinating all runtime
subsystems through an explicit, sequential startup and shutdown lifecycle:

```
xruntime.Start(appName, appFS, logDir, dataDir)
  │
  ├── 1. Resolve Application Directories (OS standards / XDG compliance or custom paths)
  ├── 2. Invalidate Outdated JSON Config Cache (compares YAML vs JSON timestamps)
  ├── 3. Create Local File System & Extract Embedded Assets (`embed.FS`)
  ├── 4. Parse YAML Config & Generate/Refresh Fast JSON Cache
  ├── 5. Substitute Dynamic Placeholders (e.g., `[[USER_DATA_DIR]]`)
  ├── 6. Validate & Initialize Database Connection (SQLite / xdb)
  ├── 7. Execute Database Schema Migrations (if DB file is newly provisioned)
  └── 8. Initialize Structured Logging (`slog` / `xlog`) & Open Session Log
```

When the application finishes execution, invoking `End()` guarantees:

- Immediate flush and closure of active session log descriptors.
- Automated log rotation based on file size (`logRegistryFileMaxSize`) or age (`logRegistryFileMaxAge`).
- Safe archiving of rotated logs with UTC/ISO timestamps.




&nbsp;
________________________________________________________________________________

## 3. EMBEDDED ASSETS LAYOUT CONVENTION

Applications powered by `xruntime` ship with default configurations and database migrations
embedded directly into the Go binary using `embed.FS`. The expected root structure inside
the embedded filesystem is:

```
appfs/
├── yaml/
│   └── config.yaml           # Source YAML configuration template
└── migrations/
    ├── 000001_init.up.sql    # Database migration scripts
    └── ...
```

Upon initial execution, `xruntime` automatically extracts these assets into the user's
local data directory (`UserDataDir`), ensuring that custom configurations and SQLite
database files persist safely across application executions.




&nbsp;
________________________________________________________________________________

## 4. BASIC USAGE

### 4.1. Application Entry Point

Initialize and defer runtime teardown cleanly within your application's `main` function:

```go
package main

import (
	"embed"
	"os"

	"github.com/AeonDigital/Go-Core-xruntime/pkg/xruntime"
)

//go:embed appfs/*
var AppFS embed.FS

func main() {
	// 1. Start runtime lifecycle (custom log/data dirs optional via env vars)
	rtConfig, err := xruntime.Start(
		"mycli",
		AppFS,
		os.Getenv("MYCLI_LOG_DIR"),
		os.Getenv("MYCLI_DATA_DIR"),
	)
	if err != nil {
		panic(err)
	}
	defer rtConfig.End()

	// 2. Access runtime services and operational metadata
	db := rtConfig.DBConfig.DB
	logger := rtConfig.Logging

	// Execute your application logic...
	_ = db
	_ = logger
}
```



&nbsp;
---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- 

### 4.2. Sample `config.yaml`

A standard `config.yaml` defines logging behaviors, directory structures, and database
parameters:

```yaml
appName: mycli
version: "1.0.0"
debug: false

logging:
  timeFormat: YYYY-MM-DD HH:mm:ss
  timeZone: UTC
  logCLI: false
  logCLILevel: all
  logRegistry: true
  logRegistryLevel: all
  logRegistryDirPath: ""        # Uses standard user log dir if empty
  logRegistryFileName: ""       # Defaults to 'current.log'
  logRegistryFileMaxSize: 2M    # Rotates log when size exceeds limit
  logRegistryFileMaxAge: 7d     # Rotates log when age exceeds threshold

dbConfig:
  driver: "sqlite"
  dsn: ""
  migrationsDirPath: "[[USER_DATA_DIR]]/appfs/migrations"
  maxOpenConnections: 1
  maxIdleConnections: 1
  connectionMaxLifetime: 30
  sqlite:
    mode: "file:"
    dir: ""                     # Defaults to UserDataDir
    fileName: "sqlite.db"
    querystring: ""
```




&nbsp;
________________________________________________________________________________

## 5. DEVELOPER REFERENCE & CONVENTIONS

### 5.1. Dynamic Configuration Variables

In your YAML configuration, you can use built-in template placeholders that `xruntime`
resolves dynamically at boot time:

- `[[USER_DATA_DIR]]` — Replaced with the resolved absolute path of `UserDataDir`.

### 5.2. Compilation & Caching Strategy

- `config.yaml` is the human-editable configuration source.
- `config.json` is automatically generated on first boot as a high-speed runtime cache.
- If `config.yaml` is modified (timestamp is newer than `config.json`), `xruntime`
  invalidates and regenerates `config.json` transparently.

### 5.3. Multi-Instance Isolation

`xruntime` does not store global singleton state. Each call to `xruntime.Start(...)`
returns an independent `*RuntimeConfig` instance, enabling concurrent execution of
isolated runtimes (e.g. in multi-tenant tools or parallel unit test suites).




&nbsp;
________________________________________________________________________________

## 6. ADDITIONAL INFORMATION

This project uses the [Semantic Versioning](https://semver.org/) system proposed
by **Tom Preston-Werner**.




&nbsp;
________________________________________________________________________________

## 7. LICENSE

This project is offered under the [MIT license](LICENSE.md).