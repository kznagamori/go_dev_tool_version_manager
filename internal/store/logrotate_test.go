package store

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

const testLogDirectory = "/data/gdtvm/logs"

// testLogMaxBytes はdocs/05-configuration.md §3.6の`max_bytes_per_file`下限である。
const testLogMaxBytes = int64(1) << 20

func logDirectoryValue(t *testing.T) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(domain.RoleLog, testLogDirectory)
	if err != nil {
		t.Fatalf("NewPathValue = %v", err)
	}
	return value
}

func logHost(t *testing.T) domain.Platform {
	t.Helper()
	platform, err := domain.ParsePlatform("linux-amd64-glibc")
	if err != nil {
		t.Fatalf("ParsePlatform = %v", err)
	}
	return platform
}

func logInvocation(t *testing.T, hex string) domain.InvocationID {
	t.Helper()
	value, err := domain.ParseInvocationID(hex)
	if err != nil {
		t.Fatalf("ParseInvocationID(%q) = %v", hex, err)
	}
	return value
}

// newLogSet はlog directoryを作ったfake setを返す。
func newLogSet(t *testing.T) *fake.Set {
	t.Helper()
	set := fake.NewSet()
	if err := set.FileSystem.MkdirAll(testLogDirectory, 0o700); err != nil {
		t.Fatalf("MkdirAll = %v", err)
	}
	return set
}

func newRotateRequest(t *testing.T, set *fake.Set) RotateLogsRequest {
	t.Helper()
	return RotateLogsRequest{
		Directory:  logDirectoryValue(t),
		Host:       logHost(t),
		Now:        time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC),
		Invocation: logInvocation(t, "33333333333333333333333333333333"),
		MaxFiles:   5,
		MaxBytes:   testLogMaxBytes,
		FileSystem: set.FileSystem,
	}
}

// putLog は指定sizeのlog fileを置く。
func putLog(t *testing.T, set *fake.Set, name string, size int) {
	t.Helper()
	set.FileSystem.AddFile(testLogDirectory+"/"+name, make([]byte, size), 0o600)
}

// logNames は`logs/`直下のfile名をbyte順で返す。
func logNames(t *testing.T, set *fake.Set) []string {
	t.Helper()
	var names []string
	err := set.FileSystem.Walk(testLogDirectory, func(_ string, info port.FileInfo) error {
		if !info.IsDir {
			names = append(names, info.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk = %v", err)
	}
	return names
}

// TestLogLineLimitFitsSmallestFileLimit は§21と§3.6の2つの上限が両立することを固定する。
//
// docs/04-storage-and-data.md §21の「log 1 line 256 KiB」が
// docs/05-configuration.md §3.6の`max_bytes_per_file`下限1 MiBを超えると、
// 新しいfileへ書いても即座に上限を超えてrotationが終わらない。片方の仕様だけを
// 動かしたときにここで気付けるようにする。
func TestLogLineLimitFitsSmallestFileLimit(t *testing.T) {
	if int64(LogLineMaxBytes) >= testLogMaxBytes {
		t.Fatalf("log 1行の上限%d byteが max_bytes_per_file 下限%d byte以上",
			LogLineMaxBytes, testLogMaxBytes)
	}
}

// TestRotateLogsKeepsFileBelowLimit は追記後に上限を超える場合だけ退避することを固定する。
func TestRotateLogsKeepsFileBelowLimit(t *testing.T) {
	tests := []struct {
		name        string
		currentSize int
		nextLine    int
		wantRotated bool
	}{
		{"現行fileが無い", 0, 100, false},
		{"追記しても上限内", int(testLogMaxBytes) - 100, 99, false},
		{"追記でちょうど上限", int(testLogMaxBytes) - 100, 100, false},
		{"追記で上限超過", int(testLogMaxBytes) - 100, 101, true},
		{"現在sizeが既に上限超過", int(testLogMaxBytes) + 1, 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := newLogSet(t)
			if test.currentSize > 0 {
				putLog(t, set, LogFileName, test.currentSize)
			}
			request := newRotateRequest(t, set)
			request.NextLineBytes = test.nextLine

			result, err := RotateLogs(request)
			if err != nil {
				t.Fatalf("RotateLogs = %s", describe(err))
			}
			if result.Rotated != test.wantRotated {
				t.Errorf("Rotated = %v, want %v", result.Rotated, test.wantRotated)
			}
			if result.CurrentPath.Path() != testLogDirectory+"/"+LogFileName {
				t.Errorf("CurrentPath = %q", result.CurrentPath.Path())
			}
			// 退避したなら現行fileは消え、退避名のfileが1件増える。
			names := logNames(t, set)
			if test.wantRotated {
				if len(names) != 1 || names[0] == LogFileName {
					t.Fatalf("退避後のfile = %v", names)
				}
				if names[0] != result.RotatedName {
					t.Errorf("RotatedName = %q, 実file = %q", result.RotatedName, names[0])
				}
			} else if result.RotatedName != "" {
				t.Errorf("未retation時にRotatedName = %q", result.RotatedName)
			}
		})
	}
}

// TestRotatedLogNameSortsByTime は退避名のbyte順が時刻順と一致することを固定する。
//
// 保持上限の削除順は名前のsortで決めるため、名前の順序が時刻順でなければ
// 新しいlogから消える。
func TestRotatedLogNameSortsByTime(t *testing.T) {
	invocation := logInvocation(t, "33333333333333333333333333333333")
	times := []time.Time{
		time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
		time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	previous := ""
	for _, moment := range times {
		name := rotatedLogName(moment, invocation)
		if previous != "" && !(previous < name) {
			t.Fatalf("名前の順序が時刻順でない: %q >= %q", previous, name)
		}
		parsed, ok := parseRotatedLogName(name)
		if !ok {
			t.Fatalf("組み立てた名前 %q をparseできない", name)
		}
		if !parsed.Equal(moment) {
			t.Errorf("parse結果 = %v, want %v", parsed, moment)
		}
		previous = name
	}

	// 非UTCの時刻を渡してもUTCへ揃える。名前の順序が実行hostのtime zoneで
	// 変わると、同じdirectoryのlogが時刻順に並ばなくなる。
	zone := time.FixedZone("JST", 9*60*60)
	local := rotatedLogName(times[0].In(zone), invocation)
	if local != rotatedLogName(times[0], invocation) {
		t.Errorf("time zoneで名前が変わった: %q", local)
	}
}

// TestRotatedLogNameIsUniquePerInvocation は同一秒の衝突を避けることを固定する。
//
// rotationはlockを取らず名前の一意性で並行安全にしている。同一秒の別processが
// 同じ名前へrenameすると、片方のlogが黙って消える。
func TestRotatedLogNameIsUniquePerInvocation(t *testing.T) {
	moment := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	left := rotatedLogName(moment, logInvocation(t, "33333333333333333333333333333333"))
	right := rotatedLogName(moment, logInvocation(t, "44444444444444444444444444444444"))
	if left == right {
		t.Fatalf("別invocationが同じ退避名 %q になった", left)
	}
}

// TestParseRotatedLogNameRejectsForeignNames は管理外fileを削除対象にしないことを
// 固定する（docs/04-storage-and-data.md §6）。
func TestParseRotatedLogNameRejectsForeignNames(t *testing.T) {
	rejects := []string{
		LogFileName,
		"gdtvm-old.log",
		"gdtvm-20260807T090000Z.log",              // 識別子が無い
		"gdtvm-20260807T090000Z-3333333.log",      // 識別子が7桁
		"gdtvm-20260807T090000Z-333333333.log",    // 識別子が9桁
		"gdtvm-20260807T090000Z-3333333G.log",     // 識別子がhexでない
		"gdtvm-20260807T090000Z-33333333.txt",     // 拡張子が違う
		"gdtvm-20261307T090000Z-33333333.log",     // 13月
		"gdtvm-notatimestamp-33333333.log",        // timestampでない
		"other-20260807T090000Z-33333333.log",     // prefixが違う
		"20260807T090000Z-33333333.log",           // prefixが無い
		".gdtvm-20260807T090000Z-33333333.log",    // 先頭にdot
		"gdtvm-20260807T090000Z-33333333.log.bak", // 別の拡張子が続く
	}
	for _, name := range rejects {
		t.Run(name, func(t *testing.T) {
			if _, ok := parseRotatedLogName(name); ok {
				t.Errorf("%q を退避fileとして受理した", name)
			}
		})
	}
	// 正当な名前は受理する。上のlistが全部を落とすだけになっていないことの確認。
	if _, ok := parseRotatedLogName("gdtvm-20260807T090000Z-33333333.log"); !ok {
		t.Error("正当な退避名を拒否した")
	}
}

// TestRotateLogsAppliesRetention は`max_files`が現行fileを含む総数であることを固定する。
func TestRotateLogsAppliesRetention(t *testing.T) {
	tests := []struct {
		name      string
		maxFiles  int
		wantKept  int
		wantFirst string
	}{
		// max_files=1は履歴を持たない設定であり、退避した1件も残さない。
		{"max_files=1", 1, 0, ""},
		{"max_files=2", 2, 1, "gdtvm-20260807T090000Z-33333333.log"},
		{"max_files=3", 3, 2, "gdtvm-20260806T090000Z-22222222.log"},
		// 保持数が実file数を上回る場合は削除しない。
		{"max_files=100", 100, 4, "gdtvm-20260804T090000Z-00000000.log"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := newLogSet(t)
			putLog(t, set, LogFileName, int(testLogMaxBytes)+1)
			// 既存の退避fileを古い順に3件置く。
			putLog(t, set, "gdtvm-20260804T090000Z-00000000.log", 10)
			putLog(t, set, "gdtvm-20260805T090000Z-11111111.log", 10)
			putLog(t, set, "gdtvm-20260806T090000Z-22222222.log", 10)

			request := newRotateRequest(t, set)
			request.MaxFiles = test.maxFiles
			result, err := RotateLogs(request)
			if err != nil {
				t.Fatalf("RotateLogs = %s", describe(err))
			}
			if !result.Rotated {
				t.Fatal("退避していない")
			}
			names := logNames(t, set)
			if len(names) != test.wantKept {
				t.Fatalf("残ったfile = %v, want %d件", names, test.wantKept)
			}
			if test.wantKept > 0 && names[0] != test.wantFirst {
				t.Errorf("最古のfile = %q, want %q", names[0], test.wantFirst)
			}
			// 削除は古い順に報告される。
			for index := 1; index < len(result.RemovedNames); index++ {
				if !(result.RemovedNames[index-1] < result.RemovedNames[index]) {
					t.Errorf("RemovedNamesが古い順でない: %v", result.RemovedNames)
				}
			}
		})
	}
}

// TestRotateLogsKeepsForeignFiles は管理外fileを保持上限の削除対象にしないことを固定する。
func TestRotateLogsKeepsForeignFiles(t *testing.T) {
	set := newLogSet(t)
	putLog(t, set, LogFileName, int(testLogMaxBytes)+1)
	putLog(t, set, "gdtvm-20260804T090000Z-00000000.log", 10)
	putLog(t, set, "gdtvm-20260805T090000Z-11111111.log", 10)
	putLog(t, set, "notes.txt", 10)
	putLog(t, set, "gdtvm-old.log", 10)
	// symlinkは退避名と一致してもgdtvmが作ったものではない。
	set.FileSystem.AddLink(testLogDirectory+"/gdtvm-20260803T090000Z-99999999.log",
		port.LinkSymlink, "/etc/passwd")

	request := newRotateRequest(t, set)
	request.MaxFiles = 1
	if _, err := RotateLogs(request); err != nil {
		t.Fatalf("RotateLogs = %s", describe(err))
	}
	names := logNames(t, set)
	want := []string{"gdtvm-20260803T090000Z-99999999.log", "gdtvm-old.log", "notes.txt"}
	if len(names) != len(want) {
		t.Fatalf("残ったfile = %v, want %v", names, want)
	}
	for index, name := range want {
		if names[index] != name {
			t.Errorf("残ったfile[%d] = %q, want %q", index, names[index], name)
		}
	}
}

// TestRotateLogsRejectsSubDirectory は平坦でないlog directoryを拒否することを固定する。
//
// 削除対象をfile名だけで決めているため、sub directoryがあると孫のfileを
// 誤って対象にしうる。前提が崩れた時点で止める。
func TestRotateLogsRejectsSubDirectory(t *testing.T) {
	set := newLogSet(t)
	putLog(t, set, LogFileName, int(testLogMaxBytes)+1)
	if err := set.FileSystem.MkdirAll(testLogDirectory+"/archive", 0o700); err != nil {
		t.Fatalf("MkdirAll = %v", err)
	}
	_, err := RotateLogs(newRotateRequest(t, set))
	if err == nil {
		t.Fatal("sub directoryがあるlog directoryが通った")
	}
	if err.Code != domain.CodeFilesystem {
		t.Errorf("code = %q, want %q", err.Code, domain.CodeFilesystem)
	}
}

// TestRotateLogsRejectsUnsafeCurrent は現行fileの位置が奪われている場合を固定する。
func TestRotateLogsRejectsUnsafeCurrent(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		set := newLogSet(t)
		if err := set.FileSystem.MkdirAll(testLogDirectory+"/"+LogFileName, 0o700); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		_, err := RotateLogs(newRotateRequest(t, set))
		if err == nil || err.Code != domain.CodePathConflict {
			t.Fatalf("RotateLogs = %v", err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		set := newLogSet(t)
		set.FileSystem.AddLink(testLogDirectory+"/"+LogFileName, port.LinkSymlink, "/etc/passwd")
		_, err := RotateLogs(newRotateRequest(t, set))
		if err == nil || err.Code != domain.CodePathUnsafe {
			t.Fatalf("RotateLogs = %v", err)
		}
	})
}

// TestRotateLogsToleratesLostRenameRace は他processが先に退避した場合を固定する。
//
// rotationはlockを取らないため、renameの取りこぼしは正常系である。errorに
// すると、並行するprocessの片方が必ず失敗する。
func TestRotateLogsToleratesLostRenameRace(t *testing.T) {
	set := newLogSet(t)
	putLog(t, set, LogFileName, int(testLogMaxBytes)+1)
	putLog(t, set, "gdtvm-20260804T090000Z-00000000.log", 10)
	set.Injector.FailOnce(fake.OpRename, fmt.Errorf("先に退避された: %w", fs.ErrNotExist))

	request := newRotateRequest(t, set)
	request.MaxFiles = 1
	result, err := RotateLogs(request)
	if err != nil {
		t.Fatalf("RotateLogs = %s", describe(err))
	}
	if result.Rotated {
		t.Error("renameに負けたのにRotatedがtrue")
	}
	// 負けても保持上限は適用する。他processが増やした退避fileを放置しない。
	if len(result.RemovedNames) != 1 || result.RemovedNames[0] != "gdtvm-20260804T090000Z-00000000.log" {
		t.Errorf("RemovedNames = %v", result.RemovedNames)
	}
}

// TestRotateLogsFailureInjection は各filesystem操作の失敗が伝わることを固定する。
func TestRotateLogsFailureInjection(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		wantCode  domain.ErrorCode
	}{
		{"stat", fake.OpStat, domain.CodeFilesystem},
		{"rename", fake.OpRename, domain.CodeFilesystem},
		{"walk", fake.OpWalk, domain.CodeFilesystem},
		{"remove", fake.OpRemove, domain.CodeFilesystem},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set := newLogSet(t)
			putLog(t, set, LogFileName, int(testLogMaxBytes)+1)
			putLog(t, set, "gdtvm-20260804T090000Z-00000000.log", 10)
			putLog(t, set, "gdtvm-20260805T090000Z-11111111.log", 10)
			set.Injector.FailOnce(test.operation, errors.New("注入した失敗"))

			request := newRotateRequest(t, set)
			request.MaxFiles = 2
			_, err := RotateLogs(request)
			if err == nil {
				t.Fatalf("%s の失敗注入が効いていない", test.name)
			}
			if err.Code != test.wantCode {
				t.Errorf("code = %q, want %q", err.Code, test.wantCode)
			}
			if err.PathRole != domain.RoleLog {
				t.Errorf("path role = %q, want %q", err.PathRole, domain.RoleLog)
			}
			// 実pathをparametersへ載せない（docs/10-security.md §9.2）。
			if len(err.Parameters) != 0 {
				t.Errorf("parametersが空でない: %v", err.Parameters)
			}
		})
	}
}

// TestRotateLogsWithoutDirectory はlog directory未作成でも失敗しないことを固定する。
//
// setupがroot layoutを作る前のlogでrotationがerrorになると、setup自体の
// 診断logが取れなくなる。
func TestRotateLogsWithoutDirectory(t *testing.T) {
	set := fake.NewSet()
	result, err := RotateLogs(newRotateRequest(t, set))
	if err != nil {
		t.Fatalf("RotateLogs = %s", describe(err))
	}
	if result.Rotated || len(result.RemovedNames) != 0 {
		t.Errorf("結果 = %+v", result)
	}
}

// TestRotateLogsRejectsInvalidRequest は不正なrequestを固定する。
func TestRotateLogsRejectsInvalidRequest(t *testing.T) {
	base := newRotateRequest(t, newLogSet(t))
	tests := []struct {
		name   string
		mutate func(*RotateLogsRequest)
	}{
		{"FileSystem未設定", func(r *RotateLogsRequest) { r.FileSystem = nil }},
		{"directory未設定", func(r *RotateLogsRequest) { r.Directory = domain.PathValue{} }},
		{"host未設定", func(r *RotateLogsRequest) { r.Host = domain.Platform{} }},
		{"時刻未設定", func(r *RotateLogsRequest) { r.Now = time.Time{} }},
		{"invocation未設定", func(r *RotateLogsRequest) { r.Invocation = domain.InvocationID{} }},
		{"max_filesが0", func(r *RotateLogsRequest) { r.MaxFiles = 0 }},
		{"max_filesが負", func(r *RotateLogsRequest) { r.MaxFiles = -1 }},
		{"追記行が負", func(r *RotateLogsRequest) { r.NextLineBytes = -1 }},
		{"追記行が上限超過", func(r *RotateLogsRequest) { r.NextLineBytes = LogLineMaxBytes + 1 }},
		{"max_bytesが1行の上限未満", func(r *RotateLogsRequest) { r.MaxBytes = LogLineMaxBytes - 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			_, err := RotateLogs(request)
			if err == nil {
				t.Fatal("RotateLogs = nil, want error")
			}
			if err.Code != domain.CodeInternal {
				t.Errorf("code = %q, want %q", err.Code, domain.CodeInternal)
			}
		})
	}
}

// TestRotateLogsRejectsUnsafePath はpath組み立てを[security.Join]へ通していることを
// 固定する（docs/04-storage-and-data.md §6・§21）。
//
// 文字列連結で組み立てると、logical path 32 KiBの上限やcomponent検査が抜ける。
func TestRotateLogsRejectsUnsafePath(t *testing.T) {
	set := newLogSet(t)
	request := newRotateRequest(t, set)
	long := "/" + strings.Repeat("a", security.LogicalPathMaxBytes)
	value, err := domain.NewPathValue(domain.RoleLog, long)
	if err != nil {
		t.Fatalf("NewPathValue = %v", err)
	}
	request.Directory = value

	_, rotateErr := RotateLogs(request)
	if rotateErr == nil {
		t.Fatal("logical path上限を超えるdirectoryが通った")
	}
	if rotateErr.Code != domain.CodePathUnsafe {
		t.Errorf("code = %q, want %q", rotateErr.Code, domain.CodePathUnsafe)
	}
	if rotateErr.PathRole != domain.RoleLog {
		t.Errorf("path role = %q, want %q", rotateErr.PathRole, domain.RoleLog)
	}
}

// TestRotateLogsIsSafeInParallel は並行実行で退避fileが失われないことを固定する。
//
// docs/02-architecture.md §11はrotationが専用lockを使うとするが、同§12のlock
// 分類6件にlogは無い。lockの代わりに、退避名がinvocationごとに一意であること、
// renameに負けた側が続行すること、削除が何度実行しても同じ結果になることで
// 並行安全にしている。その前提をここで固定する。
func TestRotateLogsIsSafeInParallel(t *testing.T) {
	const workers = 8
	set := newLogSet(t)
	putLog(t, set, LogFileName, int(testLogMaxBytes)+1)

	// requestはgoroutineの外で組み立てる。t.Fatalfはtest goroutineからしか
	// 呼べないためである。
	requests := make([]RotateLogsRequest, workers)
	for index := range requests {
		requests[index] = newRotateRequest(t, set)
		requests[index].Invocation = logInvocation(t, strings.Repeat(fmt.Sprintf("%x", index), 32))
		// 保持上限では消さない。並行実行だけを見るためである。
		requests[index].MaxFiles = 100
	}

	var group sync.WaitGroup
	results := make([]RotateLogsResult, workers)
	failures := make([]*domain.Error, workers)
	group.Add(workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			defer group.Done()
			results[index], failures[index] = RotateLogs(requests[index])
		}(index)
	}
	group.Wait()

	rotated := 0
	for index := range results {
		if failures[index] != nil {
			t.Fatalf("worker %d = %s", index, describe(failures[index]))
		}
		if results[index].Rotated {
			rotated++
		}
	}
	// 現行fileは1つしか無いため、退避に成功するのはちょうど1 workerである。
	if rotated != 1 {
		t.Errorf("退避に成功したworker数 = %d, want 1", rotated)
	}
	names := logNames(t, set)
	if len(names) != 1 {
		t.Fatalf("残ったfile = %v, want 1件", names)
	}
	if _, ok := parseRotatedLogName(names[0]); !ok {
		t.Errorf("残ったfile %q が退避名でない", names[0])
	}
}
