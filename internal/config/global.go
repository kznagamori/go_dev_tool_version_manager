package config

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// SchemaVersion はglobal/project設定のschema revisionである。
//
// docs/04-storage-and-data.md §7が「schema revisionはすべて`1`」と定める。
const SchemaVersion = 1

// ConfigFileMaxBytes はglobal/project TOML 1 fileの上限である。
//
// docs/04-storage-and-data.md §21の「global/project/setup/selection TOML各file
// 1 MiB」。利用者configで拡大できない組込み値である。
const ConfigFileMaxBytes = 1 << 20

// ColorMode は色付けの方針である（docs/05-configuration.md §3.1）。
type ColorMode string

// ColorMode のexactly 3値。
const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

var colorModes = map[ColorMode]struct{}{
	ColorAuto: {}, ColorAlways: {}, ColorNever: {},
}

// ProjectFileName はschema 1が固定するproject file名である（docs/05-configuration.md §3.3）。
//
// 同§は「`filename`はschema 1で`.gdtvm.toml`固定。他値を拒否する」と定める。
// 設定keyとして残すのは、将来keyを増やしやすい型を維持するためである。
const ProjectFileName = ".gdtvm.toml"

// 各keyの範囲。docs/05-configuration.md §3.4〜§3.6が定める。
const (
	connectTimeoutMin = time.Second
	connectTimeoutMax = 5 * time.Minute
	requestTimeoutMin = 10 * time.Second
	requestTimeoutMax = time.Hour

	cacheMaxBytesMin = int64(1) << 30 // 1 GiB
	cacheMaxBytesMax = int64(1) << 40 // 1 TiB

	logMaxFilesMin = 1
	logMaxFilesMax = 100

	logMaxBytesPerFileMin = int64(1) << 20 // 1 MiB
	logMaxBytesPerFileMax = int64(1) << 30 // 1 GiB
)

// GlobalConfig は検証済みのglobal設定である（docs/05-configuration.md §3）。
//
// 未設定keyは既定値で埋めたあとの値だけを持つ。「設定されたか」を呼出し側へ
// 見せないのは、§2の優先順位を各keyで再実装させないためである。
type GlobalConfig struct {
	Color              ColorMode
	UserDataRoot       string
	ProjectFilename    string
	StopAtVCSRoot      bool
	ConnectTimeout     time.Duration
	RequestTimeout     time.Duration
	CacheMaxBytes      int64
	AutoInstallOnUse   bool
	LogLevel           port.LogLevel
	LogMaxFiles        int
	LogMaxBytesPerFile int64
}

// DefaultGlobalConfig は組込み既定値を返す（docs/05-configuration.md §3）。
//
// global fileが存在しなくても全通常操作が動く必要があるため（同§1）、
// すべてのkeyへ既定値がある。
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Color:              ColorAuto,
		UserDataRoot:       "",
		ProjectFilename:    ProjectFileName,
		StopAtVCSRoot:      true,
		ConnectTimeout:     30 * time.Second,
		RequestTimeout:     5 * time.Minute,
		CacheMaxBytes:      10737418240,
		AutoInstallOnUse:   true,
		LogLevel:           port.LevelInfo,
		LogMaxFiles:        5,
		LogMaxBytesPerFile: 5242880,
	}
}

// globalFile はTOMLのexact key集合である。
//
// pointerで「keyが無い」と「明示的にzero値を書いた」を区別する。区別しないと、
// `stop_at_vcs_root = false`と未設定が同じ扱いになり既定値trueで上書きされる。
type globalFile struct {
	Schema      *int64            `toml:"schema"`
	Application *applicationTable `toml:"application"`
	Paths       *pathsTable       `toml:"paths"`
	Project     *projectTable     `toml:"project"`
	Network     *networkTable     `toml:"network"`
	Download    *downloadTable    `toml:"download"`
	Runtime     *runtimeTable     `toml:"runtime"`
	Logs        *logsTable        `toml:"logs"`
}

type applicationTable struct {
	Color *string `toml:"color"`
}

type pathsTable struct {
	UserDataRoot *string `toml:"user_data_root"`
}

type projectTable struct {
	Filename      *string `toml:"filename"`
	StopAtVCSRoot *bool   `toml:"stop_at_vcs_root"`
}

type networkTable struct {
	ConnectTimeout *string `toml:"connect_timeout"`
	RequestTimeout *string `toml:"request_timeout"`
}

type downloadTable struct {
	CacheMaxBytes *int64 `toml:"cache_max_bytes"`
}

type runtimeTable struct {
	AutoInstallOnUse *bool `toml:"auto_install_on_use"`
}

type logsTable struct {
	Level           *string `toml:"level"`
	MaxFiles        *int64  `toml:"max_files"`
	MaxBytesPerFile *int64  `toml:"max_bytes_per_file"`
}

// GlobalRequest はglobal設定の解析入力である。
type GlobalRequest struct {
	// Data はglobal `gdtvm.toml`のraw bytesである。
	Data []byte
	// Mode は決定済みのmodeである。`paths.user_data_root`はuser modeだけに許す。
	Mode domain.Mode
	// Host はpathのabsolute判定に使うplatformである。
	Host domain.Platform
}

// ParseGlobalConfig はglobal `gdtvm.toml`をstrictに解析する。
//
// docs/05-configuration.md §1に従い、unknown key、重複key/table、型違い、enum外、
// 上限外をすべて位置付き`E_CONFIG_INVALID`で拒否する。暗黙変換、deprecated alias、
// 環境変数による上書きを行わない。
//
// fileが存在しない場合は呼出し側が[DefaultGlobalConfig]を使う。ここでは空dataを
// 「空のfileが存在する」として扱い、`schema`必須を理由に拒否する。§3が
// 「`schema=1`はfile存在時に必須」と定めるためである。
func ParseGlobalConfig(req GlobalRequest) (GlobalConfig, *domain.Error) {
	if int64(len(req.Data)) > ConfigFileMaxBytes {
		return GlobalConfig{}, configError("config.global_file_too_large", map[string]string{
			"limit": strconv.Itoa(ConfigFileMaxBytes),
			"size":  strconv.Itoa(len(req.Data)),
		})
	}

	var file globalFile
	if err := decodeStrict(req.Data, &file); err != nil {
		return GlobalConfig{}, configError("config.global_parse_failed", map[string]string{
			"detail": err.Error(),
		})
	}
	if err := requireSchema(file.Schema, "config.global_schema_invalid"); err != nil {
		return GlobalConfig{}, err
	}

	config := DefaultGlobalConfig()
	if err := applyGlobalTables(&config, file, req); err != nil {
		return GlobalConfig{}, err
	}
	return config, nil
}

func applyGlobalTables(config *GlobalConfig, file globalFile, req GlobalRequest) *domain.Error {
	if table := file.Application; table != nil && table.Color != nil {
		mode := ColorMode(*table.Color)
		if _, ok := colorModes[mode]; !ok {
			return configError("config.color_invalid", map[string]string{"value": *table.Color})
		}
		config.Color = mode
	}

	if table := file.Paths; table != nil && table.UserDataRoot != nil {
		if err := applyUserDataRoot(config, *table.UserDataRoot, req); err != nil {
			return err
		}
	}

	if table := file.Project; table != nil {
		if table.Filename != nil && *table.Filename != ProjectFileName {
			return configError("config.project_filename_invalid", map[string]string{
				"value": *table.Filename,
			})
		}
		if table.StopAtVCSRoot != nil {
			config.StopAtVCSRoot = *table.StopAtVCSRoot
		}
	}

	if table := file.Network; table != nil {
		if err := applyDuration(&config.ConnectTimeout, table.ConnectTimeout,
			"connect_timeout", connectTimeoutMin, connectTimeoutMax); err != nil {
			return err
		}
		if err := applyDuration(&config.RequestTimeout, table.RequestTimeout,
			"request_timeout", requestTimeoutMin, requestTimeoutMax); err != nil {
			return err
		}
	}

	if table := file.Download; table != nil && table.CacheMaxBytes != nil {
		if err := applyRange64(&config.CacheMaxBytes, *table.CacheMaxBytes,
			"cache_max_bytes", cacheMaxBytesMin, cacheMaxBytesMax); err != nil {
			return err
		}
	}

	if table := file.Runtime; table != nil && table.AutoInstallOnUse != nil {
		config.AutoInstallOnUse = *table.AutoInstallOnUse
	}

	return applyLogsTable(config, file.Logs)
}

func applyLogsTable(config *GlobalConfig, table *logsTable) *domain.Error {
	if table == nil {
		return nil
	}
	if table.Level != nil {
		level, err := port.ParseLogLevel(*table.Level)
		if err != nil {
			return configError("config.log_level_invalid", map[string]string{"value": *table.Level})
		}
		config.LogLevel = level
	}
	if table.MaxFiles != nil {
		value := config.LogMaxFiles
		converted := int64(value)
		if err := applyRange64(&converted, *table.MaxFiles,
			"max_files", logMaxFilesMin, logMaxFilesMax); err != nil {
			return err
		}
		config.LogMaxFiles = int(converted)
	}
	if table.MaxBytesPerFile != nil {
		if err := applyRange64(&config.LogMaxBytesPerFile, *table.MaxBytesPerFile,
			"max_bytes_per_file", logMaxBytesPerFileMin, logMaxBytesPerFileMax); err != nil {
			return err
		}
	}
	return nil
}

// applyUserDataRoot は`paths.user_data_root`を検査する（docs/05-configuration.md §3.2）。
//
// 同§は「空または現在user所有のabsolute directory。user modeだけに許し、
// portableでは非空を拒否する」と定める。所有者、network share、symlink/reparse
// loopの検査はfilesystem操作を要するためP2-03が行う（docs/13-progress.md）。
func applyUserDataRoot(config *GlobalConfig, value string, req GlobalRequest) *domain.Error {
	if value == "" {
		return nil
	}
	if req.Mode == domain.ModePortable {
		return configError("config.user_data_root_in_portable", nil)
	}
	if req.Host.IsZero() {
		return configError("config.host_platform_missing", nil)
	}
	if !isAbsolutePath(value, pathSeparator(req.Host)) {
		return configError("config.user_data_root_not_absolute", map[string]string{"value": value})
	}
	config.UserDataRoot = value
	return nil
}

func applyDuration(target *time.Duration, raw *string, key string, minimum, maximum time.Duration) *domain.Error {
	if raw == nil {
		return nil
	}
	value, err := time.ParseDuration(*raw)
	if err != nil {
		return configError("config.duration_invalid", map[string]string{
			"key":   key,
			"value": *raw,
		})
	}
	if value < minimum || value > maximum {
		return configError("config.duration_out_of_range", map[string]string{
			"key":   key,
			"value": *raw,
			"min":   minimum.String(),
			"max":   maximum.String(),
		})
	}
	*target = value
	return nil
}

func applyRange64(target *int64, value int64, key string, minimum, maximum int64) *domain.Error {
	if value < minimum || value > maximum {
		return configError("config.value_out_of_range", map[string]string{
			"key":   key,
			"value": strconv.FormatInt(value, 10),
			"min":   strconv.FormatInt(minimum, 10),
			"max":   strconv.FormatInt(maximum, 10),
		})
	}
	*target = value
	return nil
}

func requireSchema(schema *int64, messageID string) *domain.Error {
	if schema == nil {
		return configError(messageID, map[string]string{"reason": "schema keyが無い"})
	}
	if *schema != SchemaVersion {
		return configError(messageID, map[string]string{
			"value": strconv.FormatInt(*schema, 10),
			"want":  strconv.Itoa(SchemaVersion),
		})
	}
	return nil
}

// decodeStrict はunknown keyを拒否しつつTOMLをdecodeする。
//
// 位置付きの診断を返すのはdocs/05-configuration.md §1の要求である。go-toml/v2の
// `DecodeError`と`StrictMissingError`が行・列を持つため、それをそのまま
// message parameterへ載せる。TOML 1.0は重複key/tableをparse errorにするため、
// 重複検査を実装側で追加しない。
func decodeStrict(data []byte, target any) error {
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return describeDecodeError(err)
	}
	return nil
}

func describeDecodeError(err error) error {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) {
		if len(strict.Errors) > 0 {
			first := strict.Errors[0]
			row, column := first.Position()
			return fmt.Errorf("%d行%d列: 未知のkey %q", row, column, first.Key())
		}
		return errors.New("未知のkeyがある")
	}
	var decode *toml.DecodeError
	if errors.As(err, &decode) {
		row, column := decode.Position()
		return fmt.Errorf("%d行%d列: %s", row, column, decode.Error())
	}
	return err
}
