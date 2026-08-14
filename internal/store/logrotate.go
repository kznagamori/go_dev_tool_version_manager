package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// LogFileName は現行のstructured log fileの名前である。
//
// docs/04-storage-and-data.md §2のlayoutは`logs/`だけを定め、file名を定めない。
// docs/05-configuration.md §3.6の`max_files`が複数fileを前提にするため、現行1件と
// 退避複数件という構成をここで規約として決めている。
const LogFileName = "gdtvm.log"

// 退避fileの名前は`gdtvm-<UTC timestamp>-<invocation IDの先頭8桁>.log`とする。
//
// 番号をずらすcascade（`gdtvm.1.log`→`gdtvm.2.log`…）にしない。cascadeは1回の
// rotationがN回のrenameになり、途中で中断すると番号の重複と欠落が残る。
// timestampを名前へ入れれば1回のatomic renameで完了し、名前のbyte順がそのまま
// 時刻順になる。invocation IDを付けるのは、同一秒に別processがrotationしても
// 退避先が衝突しないようにするためである。
const (
	rotatedLogPrefix = "gdtvm-"
	rotatedLogSuffix = ".log"
	// rotatedLogTimeLayout は退避名へ埋めるUTC時刻の書式である。固定幅・zero
	// paddingであるため、名前のbyte順と時刻順が一致する。
	rotatedLogTimeLayout = "20060102T150405Z"
	// rotatedLogTagLen はinvocation IDから取る識別子の桁数である。
	rotatedLogTagLen = 8
)

// RotateLogsRequest はstructured logのrotationと保持上限適用の入力である。
type RotateLogsRequest struct {
	// Directory は`logs/`のabsolute pathである。roleは`log`を使う。
	Directory domain.PathValue
	// Host はpath区切りとcomponent規則を決めるplatformである。
	//
	// runtime.GOOSではなく引数で受ける。どちらのrunnerからでも両OSの規則を
	// testできるようにするためである（CLAUDE.md §5）。
	Host domain.Platform
	// Now は退避名へ埋めるUTC時刻である。
	Now time.Time
	// Invocation は退避名の識別子に使うrequest IDである。
	Invocation domain.InvocationID
	// MaxFiles は現行fileを含めて保持するlog file数である
	// （docs/05-configuration.md §3.6の`logs.max_files`）。
	MaxFiles int
	// MaxBytes は1 log fileの上限byte数である（同§の`max_bytes_per_file`）。
	MaxBytes int64
	// NextLineBytes はこれから追記する1行のbyte数である。
	//
	// 追記後に上限を超えるかを追記前に判定するために受け取る。0なら現在size
	// だけで判定する。
	NextLineBytes int
	// FileSystem は読書きに使う。
	FileSystem port.FileSystem
}

// RotateLogsResult はrotationの結果である。
type RotateLogsResult struct {
	// CurrentPath は現行log fileのpathである。rotationの有無によらず返す。
	CurrentPath domain.PathValue
	// Rotated は現行fileを退避したかである。
	Rotated bool
	// RotatedName は退避先のfile名である。未rotationなら空である。
	RotatedName string
	// RemovedNames は保持上限で削除した退避fileの名前を古い順に持つ。
	RemovedNames []string
}

// RotateLogs は必要ならlogを退避し、保持上限を適用する。
//
// docs/10-security.md §12が「通常logの既定levelはinfo、rotation/保持上限を
// 適用する」と定め、上限値はdocs/05-configuration.md §3.6の`logs.max_files`
// （1〜100）と`max_bytes_per_file`（1 MiB〜1 GiB）である。
//
// 呼出し側は1行を追記する前に呼び、[RotateLogsResult.CurrentPath]へ追記する。
// 実際の追記は[port.Logger]の実装（P9）が行う。[port.FileSystem]はappendを
// 持たず、docs/02-architecture.md §4.1のport表にもappendが無いためである。
//
// **rotationの失敗でoperationを失敗させない。** 呼出し側はerrorを診断へ回して
// 追記を続けるか諦めるかを決める。logが書けないことでinstallを巻き戻すと、
// 診断のための機構が本体の可用性を下げることになる。
//
// docs/02-architecture.md §11はrotationが「専用lockとatomic write/delete」を
// 使うとするが、同§12のlock分類6件にlogは無い。§12はPlan transactionの取得順
// を定めるもので、logはそこへ含まれない。そのため本実装は§12のlockを取らず、
// 並行実行に対して構造で安全にする。退避先の名前がinvocationごとに一意で
// 衝突せず、renameに負けたprocessは「他がすでに退避した」として続行し、
// 保持上限の削除は同じ集合に対して何度実行しても同じ結果になる。
func RotateLogs(request RotateLogsRequest) (RotateLogsResult, *domain.Error) {
	var result RotateLogsResult
	if err := checkRotateLogsRequest(request); err != nil {
		return result, err
	}
	current, joinErr := logChildPath(request, LogFileName)
	if joinErr != nil {
		return result, logRotateError(domain.CodePathUnsafe, "log.rotate_path_unsafe", joinErr)
	}
	result.CurrentPath = current

	size, err := currentLogSize(request, current.Path())
	if err != nil {
		return result, err
	}
	if size == 0 || size+int64(request.NextLineBytes) <= request.MaxBytes {
		return result, nil
	}

	name := rotatedLogName(request.Now, request.Invocation)
	target, targetErr := logChildPath(request, name)
	if targetErr != nil {
		return result, logRotateError(domain.CodePathUnsafe, "log.rotate_path_unsafe", targetErr)
	}
	switch renameErr := request.FileSystem.Rename(current.Path(), target.Path()); {
	case renameErr == nil:
		result.Rotated = true
		result.RotatedName = name
	case isNotExistError(renameErr):
		// 他processが先に退避した。競合は解消済みであり、失敗として扱わない。
	default:
		return result, logRotateError(domain.CodeFilesystem, "log.rotate_rename_failed", renameErr)
	}

	// 退避fileが1件増えた直後だけ保持上限を適用する。1行ごとにdirectoryを
	// 走査すると、log出力の費用がfile数に比例して増える。
	// 途中まで削除して失敗した場合も削除済みの分を返す。呼出し側が「何も
	// 消えていない」と「一部消えた」を区別できるようにする。
	removed, pruneErr := pruneRotatedLogs(request)
	result.RemovedNames = removed
	if pruneErr != nil {
		return result, pruneErr
	}
	return result, nil
}

// currentLogSize は現行log fileのbyte数を返す。不在は0とする。
//
// directoryやsymlinkが同じ名前を占めている場合は拒否する。symlinkのままrename
// すると管理rootの外のfileを動かすことになり、directoryならlogを書けない状態が
// 黙って続く（docs/04-storage-and-data.md §6）。
func currentLogSize(request RotateLogsRequest, path string) (int64, *domain.Error) {
	info, err := request.FileSystem.Stat(path)
	if err != nil {
		if isNotExistError(err) {
			return 0, nil
		}
		return 0, logRotateError(domain.CodeFilesystem, "log.rotate_stat_failed", err)
	}
	if info.IsDir {
		return 0, logRotateError(domain.CodePathConflict, "log.rotate_current_not_file",
			fmt.Errorf("%s がdirectoryである", LogFileName))
	}
	if info.IsSymlink {
		return 0, logRotateError(domain.CodePathUnsafe, "log.rotate_current_not_file",
			fmt.Errorf("%s がsymlink/reparse pointである", LogFileName))
	}
	return info.Size, nil
}

// rotatedLog は退避fileの識別結果である。
type rotatedLog struct {
	name    string
	created time.Time
}

// pruneRotatedLogs は保持上限を超えた退避fileを古い順に削除する。
//
// `max_files`は現行fileを含む総数であるため、退避fileは`max_files-1`件まで
// 残す。`max_files=1`は履歴を持たない設定であり、退避した直後にその1件を消す。
func pruneRotatedLogs(request RotateLogsRequest) ([]string, *domain.Error) {
	rotated, err := listRotatedLogs(request)
	if err != nil {
		return nil, err
	}
	keep := request.MaxFiles - 1
	if len(rotated) <= keep {
		return nil, nil
	}
	var removed []string
	for _, entry := range rotated[:len(rotated)-keep] {
		path, joinErr := logChildPath(request, entry.name)
		if joinErr != nil {
			return removed, logRotateError(domain.CodePathUnsafe, "log.rotate_path_unsafe", joinErr)
		}
		if removeErr := request.FileSystem.Remove(path.Path()); removeErr != nil {
			return removed, logRotateError(domain.CodeFilesystem, "log.rotate_remove_failed", removeErr)
		}
		removed = append(removed, entry.name)
	}
	return removed, nil
}

// listRotatedLogs は`logs/`直下の退避fileを古い順に返す。
//
// 判定はfile名だけで行う。file名だけで削除対象を決められるのは`logs/`が
// docs/04-storage-and-data.md §2のlayoutで平坦なためであり、sub directoryが
// あればその前提が崩れるので拒否する。同§6が「削除は所有を証明できるものだけ」
// と定めるため、gdtvmが作った名前でないfileとsymlinkは対象にしない。
func listRotatedLogs(request RotateLogsRequest) ([]rotatedLog, *domain.Error) {
	var found []rotatedLog
	directories := 0
	walkErr := request.FileSystem.Walk(request.Directory.Path(),
		func(_ string, info port.FileInfo) error {
			if info.IsDir {
				directories++
				// Walkはroot自身も報告する。root以外のdirectoryは管理外である。
				if directories > 1 {
					return fmt.Errorf("log directoryにsub directory %q がある", info.Name)
				}
				return nil
			}
			if info.IsSymlink {
				return nil
			}
			if created, ok := parseRotatedLogName(info.Name); ok {
				found = append(found, rotatedLog{name: info.Name, created: created})
			}
			return nil
		})
	if walkErr != nil {
		// directoryが未作成なら退避fileも無い。setupが作る前のlogは現行fileへ
		// 書けないため、ここで作らずrotation対象なしとして返す。
		if isNotExistError(walkErr) {
			return nil, nil
		}
		return nil, logRotateError(domain.CodeFilesystem, "log.rotate_list_failed", walkErr)
	}
	// 名前は固定幅のUTC timestampで始まるため、byte順がそのまま時刻順になる。
	// 同一秒はinvocation IDで決まり、順序は実行ごとに変わらない。
	sort.Slice(found, func(left, right int) bool { return found[left].name < found[right].name })
	return found, nil
}

// rotatedLogName は退避先のfile名を組み立てる。
func rotatedLogName(now time.Time, invocation domain.InvocationID) string {
	return rotatedLogPrefix + now.UTC().Format(rotatedLogTimeLayout) +
		"-" + invocation.String()[:rotatedLogTagLen] + rotatedLogSuffix
}

// parseRotatedLogName はfile名が退避fileの規約に一致するかを判定する。
//
// 一致した場合だけ削除対象にする。前方一致だけで判定すると、利用者が置いた
// `gdtvm-old.log`のようなfileを消してしまう。
func parseRotatedLogName(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, rotatedLogPrefix) || !strings.HasSuffix(name, rotatedLogSuffix) {
		return time.Time{}, false
	}
	middle := name[len(rotatedLogPrefix) : len(name)-len(rotatedLogSuffix)]
	stamp, tag, found := strings.Cut(middle, "-")
	if !found || len(tag) != rotatedLogTagLen {
		return time.Time{}, false
	}
	for _, char := range tag {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return time.Time{}, false
		}
	}
	created, err := time.Parse(rotatedLogTimeLayout, stamp)
	if err != nil || created.IsZero() {
		return time.Time{}, false
	}
	return created, true
}

// logChildPath は`logs/`直下のpathを組み立てる。
//
// 文字列連結ではなく[security.Join]を通す。docs/04-storage-and-data.md §6の
// component検査（区切り混在、`..`、予約名、ADS、NUL）を通さない経路を作ると、
// file名の由来が変わったときに検査が抜ける。
func logChildPath(request RotateLogsRequest, name string) (domain.PathValue, error) {
	return security.Join(security.JoinRequest{
		Root: request.Directory, Host: request.Host, Components: []string{name},
	})
}

func checkRotateLogsRequest(request RotateLogsRequest) *domain.Error {
	invalid := func(reason string) *domain.Error {
		return logRotateError(domain.CodeInternal, "log.rotate_request_invalid", errors.New(reason))
	}
	switch {
	case request.FileSystem == nil:
		return invalid("FileSystemが未設定")
	case request.Directory.IsZero() || request.Directory.Path() == "":
		return invalid("log directoryが未設定")
	case request.Host.IsZero():
		return invalid("host platformが未設定")
	case request.Now.IsZero():
		return invalid("現在時刻が未設定")
	case request.Invocation.IsZero():
		return invalid("invocation IDが未設定")
	case request.MaxFiles < 1:
		return invalid(fmt.Sprintf("max_filesは1以上（%d）", request.MaxFiles))
	case request.NextLineBytes < 0 || request.NextLineBytes > LogLineMaxBytes:
		return invalid(fmt.Sprintf("追記行が%d byteの範囲外（%d byte）",
			LogLineMaxBytes, request.NextLineBytes))
	// 1行の上限（§21の256 KiB）より小さいmax_bytes_per_fileを許すと、新しいfileへ
	// 書いても即座に上限を超え、rotationが終わらない。§3.6の下限が1 MiBである
	// ため通常は成立するが、成立しない値を黙って受けない。
	case request.MaxBytes < LogLineMaxBytes:
		return invalid(fmt.Sprintf("max_bytes_per_fileがlog 1行の上限%d byteを下回る（%d byte）",
			LogLineMaxBytes, request.MaxBytes))
	}
	return nil
}

func logRotateError(code domain.ErrorCode, messageID string, cause error) *domain.Error {
	return typedError(code, messageID, domain.RoleLog, cause)
}
