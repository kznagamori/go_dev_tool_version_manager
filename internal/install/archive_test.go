package install

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func platformOf(t *testing.T, id string) domain.Platform {
	t.Helper()
	value, err := domain.ParsePlatform(id)
	if err != nil {
		t.Fatalf("ParsePlatform(%q): %v", id, err)
	}
	return value
}

// goArchive はGoのarchiveを模したentry列である（top-level `go/`を1件除去する）。
func goArchive() []Entry {
	return []Entry{
		{Name: "go/", Kind: KindDir},
		{Name: "go/bin/", Kind: KindDir},
		{Name: "go/bin/go", Kind: KindFile, Size: 1000, CompressedSize: 400},
		{Name: "go/bin/gofmt", Kind: KindFile, Size: 800, CompressedSize: 300},
	}
}

// TestInspectEntriesAcceptsStandardLayout は標準的なarchiveが通ることを固定する。
func TestInspectEntriesAcceptsStandardLayout(t *testing.T) {
	for _, id := range []string{"windows-amd64", "linux-amd64-glibc"} {
		t.Run(id, func(t *testing.T) {
			result, err := InspectEntriesResult(InspectRequest{
				Entries: goArchive(), StripComponents: 1, Host: platformOf(t, id),
			})
			if err != nil {
				t.Fatalf("InspectEntriesResult = %v", err.Cause)
			}
			want := []string{"bin", "bin/go", "bin/gofmt"}
			if len(result.Paths) != len(want) {
				t.Fatalf("Paths = %v, want %v", result.Paths, want)
			}
			for index := range want {
				if result.Paths[index] != want[index] {
					t.Errorf("Paths[%d] = %q, want %q", index, result.Paths[index], want[index])
				}
			}
			if result.TotalBytes != 1800 {
				t.Errorf("TotalBytes = %d, want 1800", result.TotalBytes)
			}
		})
	}
}

// TestInspectEntriesStripComponentsZero は`strip_components=0`でpathをそのまま
// 使うことを固定する（.NET SDKのarchiveはtop-level directoryを持たない）。
func TestInspectEntriesStripComponentsZero(t *testing.T) {
	entries := []Entry{
		{Name: "dotnet", Kind: KindFile, Size: 100, CompressedSize: 50},
		{Name: "sdk/", Kind: KindDir},
		{Name: "sdk/8.0.400/", Kind: KindDir},
	}
	result, err := InspectEntriesResult(InspectRequest{
		Entries: entries, StripComponents: 0, Host: platformOf(t, "linux-amd64-glibc"),
	})
	if err != nil {
		t.Fatalf("InspectEntriesResult = %v", err.Cause)
	}
	want := []string{"dotnet", "sdk", "sdk/8.0.400"}
	for index := range want {
		if result.Paths[index] != want[index] {
			t.Errorf("Paths[%d] = %q, want %q", index, result.Paths[index], want[index])
		}
	}
}

// TestInspectEntriesRejectsUnsafeName はdocs/10-security.md §5が挙げる
// path違反をすべて拒否することを固定する。
//
// **1件でも違反があれば展開しない。** 安全なentryだけを選んで展開すると、
// archiveが意図した構成と違うものが出来上がる。
func TestInspectEntriesRejectsUnsafeName(t *testing.T) {
	cases := []struct{ name, entry string }{
		{"absolute", "/etc/passwd"},
		{"drive letter", "C:/windows/system32"},
		{"drive letter小文字", "c:/x"},
		{"UNC", "//server/share/x"},
		{"親参照", "../outside"},
		{"途中の親参照", "go/../../outside"},
		{"カレント参照", "./x"},
		{"途中のカレント参照", "go/./x"},
		{"空component", "go//x"},
		{"backslash", `go\bin\go.exe`},
		{"nameが空", ""},
		{"rootだけ", "/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries := append(goArchive(), Entry{Name: c.entry, Kind: KindFile, Size: 1})
			err := InspectEntries(InspectRequest{
				Entries: entries, StripComponents: 1, Host: platformOf(t, "linux-amd64-glibc"),
			})
			if err == nil {
				t.Fatalf("危険なname %q が通った", c.entry)
			}
			if err.Code != domain.CodeArchiveUnsafe {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeArchiveUnsafe)
			}
			// 同じarchiveを展開し直しても同じ結果になる（§14）。
			if err.Retryable {
				t.Error("archive安全検査の失敗がretryable=trueになっている")
			}
		})
	}
}

// TestInspectEntriesRejectsWindowsOnlyNames はWindows hostでだけ拒否する
// name規則を固定する。
//
// ADSの`:`、末尾の空白/dot、予約device名はLinuxでは通常のfile名として有効であり、
// 一律に拒否すると正当なarchiveを扱えなくなる。判定はhostで分ける
// （`internal/security.ValidateComponent`と同じ方針）。
func TestInspectEntriesRejectsWindowsOnlyNames(t *testing.T) {
	cases := []struct{ name, entry string }{
		{"ADS", "go/bin/go.exe:stream"},
		{"末尾空白", "go/bin /x"},
		{"末尾dot", "go/bin./x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries := append(goArchive(), Entry{Name: c.entry, Kind: KindFile, Size: 1})
			if err := InspectEntries(InspectRequest{
				Entries: entries, StripComponents: 1, Host: platformOf(t, "windows-amd64"),
			}); err == nil {
				t.Fatalf("Windowsで %q が通った", c.entry)
			}
			// Linuxでは有効なname である。
			if err := InspectEntries(InspectRequest{
				Entries: entries, StripComponents: 1, Host: platformOf(t, "linux-amd64-glibc"),
			}); err != nil {
				t.Errorf("Linuxで %q が拒否された: %v", c.entry, err.Cause)
			}
		})
	}
}

// TestInspectEntriesRejectsControlAndNonNFC は制御文字とnon-NFCを拒否することを
// 固定する。
//
// 正規化して受けると、archiveが宣言したnameと展開先のnameが違う状態になり、
// probeのrequired pathと突き合わせられない。
func TestInspectEntriesRejectsControlAndNonNFC(t *testing.T) {
	cases := []struct{ name, entry string }{
		{"NUL", "go/bin/a\x00b"},
		{"制御文字", "go/bin/a\x01b"},
		{"DEL", "go/bin/a\x7fb"},
		{"改行", "go/bin/a\nb"},
		{"format制御文字", "go/bin/a\u200eb"},
		// NFD（結合文字）はNFCへ正規化されていない。
		{"non-NFC", "go/bin/cafe\u0301"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries := append(goArchive(), Entry{Name: c.entry, Kind: KindFile, Size: 1})
			if err := InspectEntries(InspectRequest{
				Entries: entries, StripComponents: 1, Host: platformOf(t, "linux-amd64-glibc"),
			}); err == nil {
				t.Fatalf("%q が通った", c.entry)
			}
		})
	}

	// NFCそのものは通る。非ASCIIを一律に拒否しているわけではない。
	entries := append(goArchive(), Entry{Name: "go/bin/caf\u00e9", Kind: KindFile, Size: 1})
	if err := InspectEntries(InspectRequest{
		Entries: entries, StripComponents: 1, Host: platformOf(t, "linux-amd64-glibc"),
	}); err != nil {
		t.Errorf("NFC正規化済みの非ASCII nameが拒否された: %v", err.Cause)
	}
}

// TestInspectEntriesRejectsLinksAndSpecialFiles はsymlink/hardlink/特殊fileを
// 拒否することを固定する。
//
// 展開先の外を指せるため、archiveの中に限っても許さない。
func TestInspectEntriesRejectsLinksAndSpecialFiles(t *testing.T) {
	for _, kind := range []EntryKind{KindSymlink, KindHardlink, KindOther} {
		t.Run(string(kind), func(t *testing.T) {
			entries := append(goArchive(), Entry{Name: "go/bin/link", Kind: kind})
			if err := InspectEntries(InspectRequest{
				Entries: entries, StripComponents: 1, Host: platformOf(t, "linux-amd64-glibc"),
			}); err == nil {
				t.Fatalf("%s entryが通った", kind)
			}
		})
	}
	t.Run("未知の種別", func(t *testing.T) {
		entries := append(goArchive(), Entry{Name: "go/bin/x", Kind: EntryKind("weird")})
		if err := InspectEntries(InspectRequest{
			Entries: entries, StripComponents: 1, Host: platformOf(t, "linux-amd64-glibc"),
		}); err == nil {
			t.Fatal("未知の種別が通った")
		}
	})
}

// TestInspectEntriesRejectsDuplicateAndCaseCollision は重複とcase衝突を拒否する
// ことを固定する。
//
// Windowsはcase insensitiveなので`Bin/go`と`bin/go`が同じfileを指す。Linuxでも
// 衝突させないのは、同じarchiveを両OSで同じ構成へ展開するためである。
func TestInspectEntriesRejectsDuplicateAndCaseCollision(t *testing.T) {
	cases := []struct{ name, entry string }{
		{"完全重複", "go/bin/go"},
		{"case違い", "go/bin/GO"},
		{"directoryのcase違い", "go/BIN/other"},
	}
	for _, c := range cases {
		for _, id := range []string{"windows-amd64", "linux-amd64-glibc"} {
			t.Run(c.name+"/"+id, func(t *testing.T) {
				entries := append(goArchive(), Entry{Name: c.entry, Kind: KindFile, Size: 1})
				if err := InspectEntries(InspectRequest{
					Entries: entries, StripComponents: 1, Host: platformOf(t, id),
				}); err == nil {
					t.Fatalf("%q が通った", c.entry)
				}
			})
		}
	}
}

// TestInspectEntriesRejectsWindowsReservedNames はWindows予約device名を拒否する
// ことを固定する。
//
// 拡張子を付けても予約は解けない。
func TestInspectEntriesRejectsWindowsReservedNames(t *testing.T) {
	reserved := []string{"con", "PRN", "aux", "NUL", "com1", "lpt9", "con.txt", "COM1.exe"}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			entries := append(goArchive(), Entry{Name: "go/bin/" + name, Kind: KindFile, Size: 1})
			if err := InspectEntries(InspectRequest{
				Entries: entries, StripComponents: 1, Host: platformOf(t, "windows-amd64"),
			}); err == nil {
				t.Fatalf("Windows予約名 %q が通った", name)
			}
		})
	}
	// Linuxでは予約名ではない。同じarchiveを両OSで扱うため、判定はhostで分ける。
	entries := append(goArchive(), Entry{Name: "go/bin/con", Kind: KindFile, Size: 1})
	if err := InspectEntries(InspectRequest{
		Entries: entries, StripComponents: 1, Host: platformOf(t, "linux-amd64-glibc"),
	}); err != nil {
		t.Errorf("Linuxで`con`が拒否された: %v", err.Cause)
	}
}

// TestInspectEntriesEnforcesLimits はdocs/04-storage-and-data.md §21の上限を
// 固定する。
func TestInspectEntriesEnforcesLimits(t *testing.T) {
	host := platformOf(t, "linux-amd64-glibc")

	t.Run("entry数", func(t *testing.T) {
		entries := make([]Entry, ArchiveEntryMax+1)
		for index := range entries {
			entries[index] = Entry{
				Name: "go/f" + strings.Repeat("0", 0) + itoa(index), Kind: KindFile, Size: 1}
		}
		if err := InspectEntries(InspectRequest{
			Entries: entries, StripComponents: 1, Host: host,
		}); err == nil {
			t.Fatal("entry数の上限超過が通った")
		}
	})

	t.Run("単一fileの上限", func(t *testing.T) {
		entries := append(goArchive(), Entry{
			Name: "go/big", Kind: KindFile, Size: ArchiveFileMaxBytes + 1})
		if err := InspectEntries(InspectRequest{
			Entries: entries, StripComponents: 1, Host: host,
		}); err == nil {
			t.Fatal("単一fileの上限超過が通った")
		}
		// 上限ちょうどは通る。
		exact := append(goArchive(), Entry{
			Name: "go/big", Kind: KindFile, Size: ArchiveFileMaxBytes})
		if err := InspectEntries(InspectRequest{
			Entries: exact, StripComponents: 1, Host: host,
		}); err != nil {
			t.Errorf("上限ちょうどが拒否された: %v", err.Cause)
		}
	})

	t.Run("総展開の上限", func(t *testing.T) {
		// 加算overflowをfail closedで扱う（§21）。
		entries := []Entry{
			{Name: "go/", Kind: KindDir},
			{Name: "go/a", Kind: KindFile, Size: ArchiveFileMaxBytes},
			{Name: "go/b", Kind: KindFile, Size: ArchiveFileMaxBytes},
			{Name: "go/c", Kind: KindFile, Size: ArchiveFileMaxBytes},
			{Name: "go/d", Kind: KindFile, Size: ArchiveFileMaxBytes},
			{Name: "go/e", Kind: KindFile, Size: ArchiveFileMaxBytes},
			{Name: "go/f", Kind: KindFile, Size: ArchiveFileMaxBytes},
		}
		if err := InspectEntries(InspectRequest{
			Entries: entries, StripComponents: 1, Host: host,
		}); err == nil {
			t.Fatal("総展開の上限超過が通った")
		}
	})

	t.Run("entryの圧縮比", func(t *testing.T) {
		entries := append(goArchive(), Entry{
			Name: "go/bomb", Kind: KindFile,
			Size: int64(ArchiveRatioMax+1) * 1000, CompressedSize: 1000})
		if err := InspectEntries(InspectRequest{
			Entries: entries, StripComponents: 1, Host: host,
		}); err == nil {
			t.Fatal("圧縮比の上限超過が通った")
		}
	})

	t.Run("圧縮後sizeが不明なら判定しない", func(t *testing.T) {
		// tarのentryは個別の圧縮後sizeを持たない。
		entries := append(goArchive(), Entry{
			Name: "go/tarentry", Kind: KindFile, Size: 1 << 20, CompressedSize: 0})
		if err := InspectEntries(InspectRequest{
			Entries: entries, StripComponents: 1, Host: host,
		}); err != nil {
			t.Errorf("圧縮後size不明のentryが拒否された: %v", err.Cause)
		}
	})
}

// TestInspectEntriesRejectsInvalidRequest は要求の前提を固定する。
func TestInspectEntriesRejectsInvalidRequest(t *testing.T) {
	host := platformOf(t, "linux-amd64-glibc")
	cases := []struct {
		name string
		req  InspectRequest
	}{
		{"entryが0件", InspectRequest{Entries: nil, Host: host}},
		{"stripが負", InspectRequest{Entries: goArchive(), StripComponents: -1, Host: host}},
		// v0.1が許すstripは0か1だけである（docs/06-tool-definition.md §9）。
		{"stripが2", InspectRequest{Entries: goArchive(), StripComponents: 2, Host: host}},
		{"sizeが負", InspectRequest{
			Entries: []Entry{{Name: "go/a", Kind: KindFile, Size: -1}},
			Host:    host}},
		{"圧縮後sizeが負", InspectRequest{
			Entries: []Entry{{Name: "go/a", Kind: KindFile, CompressedSize: -1}},
			Host:    host}},
		// top-level directoryだけのarchiveはstrip後に何も残らない。
		{"strip後に空", InspectRequest{
			Entries:         []Entry{{Name: "go/", Kind: KindDir}},
			StripComponents: 1, Host: host}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := InspectEntries(c.req); err == nil {
				t.Fatal("不正な要求が通った")
			}
		})
	}
}

// itoa は依存を増やさずに整数を10進文字列にする。
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[index:])
}
