package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"hash/crc32"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
)

const (
	stagingDir  = "/data/tmp/operations/op1"
	payloadDir  = stagingDir + "/payload"
	archiveFile = cacheDir + "/tool-1.0.0.archive"
)

// zipItem はbuildZipへ渡す1 entryである。
type zipItem struct {
	name string
	data []byte
	mode fs.FileMode
}

// buildZip は宣言と実体が一致するzipを作る。
func buildZip(t *testing.T, items []zipItem) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, item := range items {
		header := &zip.FileHeader{Name: item.name, Method: zip.Deflate}
		header.SetMode(item.mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("CreateHeader(%q): %v", item.name, err)
		}
		if _, err := entry.Write(item.data); err != nil {
			t.Fatalf("Write(%q): %v", item.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}
	return buffer.Bytes()
}

// buildLyingZip は宣言sizeより多く展開されるzipを作る。
//
// 宣言を偽るzip bombを再現する。zip.Writerの通常経路は宣言と実体を必ず
// 一致させるため、header値を自分で決められるCreateRawを使う。
func buildLyingZip(t *testing.T, name string, payload []byte, declared uint64) []byte {
	t.Helper()
	var compressed bytes.Buffer
	deflater, err := flate.NewWriter(&compressed, flate.BestCompression)
	if err != nil {
		t.Fatalf("flate.NewWriter: %v", err)
	}
	if _, err := deflater.Write(payload); err != nil {
		t.Fatalf("deflate Write: %v", err)
	}
	if err := deflater.Close(); err != nil {
		t.Fatalf("deflate Close: %v", err)
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	header := &zip.FileHeader{
		Name:               name,
		Method:             zip.Deflate,
		CRC32:              crc32.ChecksumIEEE(payload),
		CompressedSize64:   uint64(compressed.Len()),
		UncompressedSize64: declared,
	}
	header.SetMode(0o644)
	entry, err := writer.CreateRaw(header)
	if err != nil {
		t.Fatalf("CreateRaw: %v", err)
	}
	if _, err := entry.Write(compressed.Bytes()); err != nil {
		t.Fatalf("raw Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}
	return buffer.Bytes()
}

// tarItem はbuildTarGzへ渡す1 entryである。
type tarItem struct {
	name     string
	data     []byte
	mode     int64
	typeflag byte
}

// buildTarGz はtar.gzを作る。
func buildTarGz(t *testing.T, items []tarItem) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, item := range items {
		header := &tar.Header{
			Name:     item.name,
			Mode:     item.mode,
			Size:     int64(len(item.data)),
			Typeflag: item.typeflag,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%q): %v", item.name, err)
		}
		if len(item.data) > 0 {
			if _, err := tarWriter.Write(item.data); err != nil {
				t.Fatalf("Write(%q): %v", item.name, err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return buffer.Bytes()
}

// goZip はGoのarchiveを模したzipである（top-level `go/`を1件除去する）。
func goZip(t *testing.T) []byte {
	t.Helper()
	return buildZip(t, []zipItem{
		{name: "go/", mode: fs.ModeDir | 0o755},
		{name: "go/bin/", mode: fs.ModeDir | 0o755},
		{name: "go/bin/go", data: bytes.Repeat([]byte("g"), 64), mode: 0o755},
		{name: "go/VERSION", data: []byte("go1.25.0"), mode: 0o644},
	})
}

// goTarGz はgoZipと同じ構成のtar.gzである。
func goTarGz(t *testing.T) []byte {
	t.Helper()
	return buildTarGz(t, []tarItem{
		{name: "go/", mode: 0o755, typeflag: tar.TypeDir},
		{name: "go/bin/", mode: 0o755, typeflag: tar.TypeDir},
		{name: "go/bin/go", data: bytes.Repeat([]byte("g"), 64), mode: 0o755, typeflag: tar.TypeReg},
		{name: "go/VERSION", data: []byte("go1.25.0"), mode: 0o644, typeflag: tar.TypeReg},
	})
}

// extractHarness は展開1件分のfakeをまとめる。
type extractHarness struct {
	fs        *fake.FileSystem
	injector  *fake.Injector
	sink      *recordingSink
	extractor *Extractor
}

func newExtractHarness(t *testing.T, archive []byte) *extractHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	filesystem.AddDir(stagingDir, 0o700)
	filesystem.AddFile(archiveFile, archive, 0o600)
	sink := &recordingSink{}
	extractor, err := NewExtractor(filesystem, progress.NewReporter(sink))
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}
	return &extractHarness{fs: filesystem, injector: injector, sink: sink, extractor: extractor}
}

// request は既定の展開要求を返す。
func (h *extractHarness) request(t *testing.T, format definition.ArchiveFormat, strip int, osID string) ExtractRequest {
	t.Helper()
	archive, err := domain.NewPathValue(domain.RoleDownloadCache, archiveFile)
	if err != nil {
		t.Fatalf("NewPathValue(archive): %v", err)
	}
	staging, err := domain.NewPathValue(domain.RoleStaging, stagingDir)
	if err != nil {
		t.Fatalf("NewPathValue(staging): %v", err)
	}
	dest, err := domain.NewPathValue(domain.RolePayload, payloadDir)
	if err != nil {
		t.Fatalf("NewPathValue(dest): %v", err)
	}
	return ExtractRequest{
		ArchivePath:     archive,
		Format:          format,
		StagingRoot:     staging,
		Dest:            dest,
		StripComponents: strip,
		Host:            platformOf(t, osID),
	}
}

// mustRead は展開先のfileを読む。
func (h *extractHarness) mustRead(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := h.fs.ReadFile(payloadDir+"/"+rel, 0)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", rel, err)
	}
	return data
}

// missing は展開先にentryが無いことを確認する。
func (h *extractHarness) missing(t *testing.T, rel string) {
	t.Helper()
	if _, err := h.fs.Stat(payloadDir + "/" + rel); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%q が残っている（err = %v）", rel, err)
	}
}

// TestExtractZipCreatesTree はzipが宣言どおりの構成へ展開されることを固定する。
func TestExtractZipCreatesTree(t *testing.T) {
	for _, id := range []string{"linux-amd64-glibc", "windows-amd64"} {
		t.Run(id, func(t *testing.T) {
			harness := newExtractHarness(t, goZip(t))
			result, err := harness.extractor.Extract(
				context.Background(), harness.request(t, definition.FormatZip, 1, id))
			if err != nil {
				t.Fatalf("Extract = %v", err.Cause)
			}

			want := []string{"bin", "bin/go", "VERSION"}
			if len(result.Paths) != len(want) {
				t.Fatalf("Paths = %v, want %v", result.Paths, want)
			}
			for index := range want {
				if result.Paths[index] != want[index] {
					t.Errorf("Paths[%d] = %q, want %q", index, result.Paths[index], want[index])
				}
			}
			if result.FileCount != 2 || result.DirCount != 1 {
				t.Errorf("FileCount/DirCount = %d/%d, want 2/1", result.FileCount, result.DirCount)
			}
			if result.TotalBytes != 72 {
				t.Errorf("TotalBytes = %d, want 72", result.TotalBytes)
			}
			if got := string(harness.mustRead(t, "VERSION")); got != "go1.25.0" {
				t.Errorf("VERSION = %q, want %q", got, "go1.25.0")
			}
			if got := len(harness.mustRead(t, "bin/go")); got != 64 {
				t.Errorf("bin/go = %d byte, want 64", got)
			}
		})
	}
}

// TestExtractTarGzCreatesTree はtar.gzでも同じ構成になることを固定する。
//
// 同じtoolを両formatで扱うため、formatの違いが展開後の構成へ出てはならない。
func TestExtractTarGzCreatesTree(t *testing.T) {
	harness := newExtractHarness(t, goTarGz(t))
	result, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatTarGz, 1, "linux-amd64-glibc"))
	if err != nil {
		t.Fatalf("Extract = %v", err.Cause)
	}
	want := []string{"bin", "bin/go", "VERSION"}
	for index := range want {
		if result.Paths[index] != want[index] {
			t.Errorf("Paths[%d] = %q, want %q", index, result.Paths[index], want[index])
		}
	}
	if result.TotalBytes != 72 {
		t.Errorf("TotalBytes = %d, want 72", result.TotalBytes)
	}
	if got := string(harness.mustRead(t, "VERSION")); got != "go1.25.0" {
		t.Errorf("VERSION = %q, want %q", got, "go1.25.0")
	}
}

// TestExtractStripComponentsZero はtop-level directoryを持たないarchiveを固定する。
func TestExtractStripComponentsZero(t *testing.T) {
	archive := buildZip(t, []zipItem{
		{name: "dotnet", data: []byte("bin"), mode: 0o755},
		{name: "sdk/", mode: fs.ModeDir | 0o755},
		{name: "sdk/README", data: []byte("x"), mode: 0o644},
	})
	harness := newExtractHarness(t, archive)
	result, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 0, "linux-amd64-glibc"))
	if err != nil {
		t.Fatalf("Extract = %v", err.Cause)
	}
	want := []string{"dotnet", "sdk", "sdk/README"}
	for index := range want {
		if result.Paths[index] != want[index] {
			t.Errorf("Paths[%d] = %q, want %q", index, result.Paths[index], want[index])
		}
	}
}

// TestExtractNormalizesPermissions はpermission正規化を固定する。
//
// docs/08-install-runtime.md §6「permissionを正規化し、Linux executableの
// owner executeを保持しsetuid/setgidを除去する」。archiveのmodeをそのまま
// 運ばず、executableかどうかだけを引き継ぐ。
func TestExtractNormalizesPermissions(t *testing.T) {
	archive := buildZip(t, []zipItem{
		// setuid＋group/other書込み可のexecutable、およびworld writableな通常file。
		{name: "bin/", mode: fs.ModeDir | 0o777},
		{name: "bin/tool", data: []byte("x"), mode: fs.ModeSetuid | 0o777},
		{name: "data.txt", data: []byte("y"), mode: 0o666},
	})
	harness := newExtractHarness(t, archive)
	if _, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 0, "linux-amd64-glibc")); err != nil {
		t.Fatalf("Extract = %v", err.Cause)
	}

	tests := []struct {
		rel  string
		want fs.FileMode
	}{
		{"bin", extractDirPerm},
		{"bin/tool", extractExecPerm},
		{"data.txt", extractFilePerm},
	}
	for _, test := range tests {
		info, err := harness.fs.Stat(payloadDir + "/" + test.rel)
		if err != nil {
			t.Fatalf("Stat(%q): %v", test.rel, err)
		}
		if got := info.Mode.Perm(); got != test.want {
			t.Errorf("%q perm = %o, want %o", test.rel, got, test.want)
		}
		if info.Mode&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
			t.Errorf("%q にsetuid/setgidが残っている（mode = %v）", test.rel, info.Mode)
		}
	}
}

// TestExtractCutsOffLyingEntry は宣言sizeを偽るzip bombを打ち切ることを固定する。
//
// 事前検査は宣言sizeしか見られない。展開中に打ち切らないと、10 byteと宣言した
// entryが何GiBでも書けてしまう。
//
// この経路は`archive/zip`のchecksumReaderが先に`zip.ErrFormat`で止める
// （読んだbytesがUncompressedSize64を超えた時点で判定する）。**stdlibが
// 止めることをこちらの安全性の根拠にしない**ため、同じ超過を自前で検出する
// 側は[TestExtractReaderCutsOffDeclaredSize]で固定する。
func TestExtractCutsOffLyingEntry(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), 1<<20)
	archive := buildLyingZip(t, "big.bin", payload, 10)
	harness := newExtractHarness(t, archive)

	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 0, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("宣言を偽るarchiveが通った")
	}
	if err.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
	}
	harness.missing(t, "big.bin")
}

// TestExtractReaderCutsOffDeclaredSize は宣言sizeを超えた実bytesを自前で
// 打ち切ることを固定する。
//
// zip/tarのreaderはどちらも宣言sizeで読取りを止めるが、それに依存すると
// docs/10-security.md §5の上限をstdlibの実装へ委ねることになる。
func TestExtractReaderCutsOffDeclaredSize(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	writer := newTestWriter(t, harness, ArchiveTotalMaxBytes)
	reader := &extractReader{
		inner:  bytes.NewReader(bytes.Repeat([]byte("A"), 100)),
		ctx:    context.Background(),
		writer: writer,
		item:   InspectedEntry{Path: "big.bin", Kind: KindFile, Size: 10},
	}
	if _, err := io.ReadAll(reader); !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("err = %v, want errArchiveTooLarge", err)
	}
}

// TestExtractRejectsShortEntry は宣言より少ないbytesしか無いentryを拒否する。
//
// 宣言と実体が食い違うarchiveを通すと、事前検査が見た総量が実体と無関係になる。
func TestExtractRejectsShortEntry(t *testing.T) {
	// 宣言100 byte、実体は10 byte。
	archive := buildLyingZip(t, "short.bin", bytes.Repeat([]byte("A"), 10), 100)
	harness := newExtractHarness(t, archive)

	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 0, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("宣言より短いarchiveが通った")
	}
	if err.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
	}
	harness.missing(t, "short.bin")
}

// TestExtractRejectsCorruptContent は壊れたentryをarchive側の失敗として扱う。
//
// CRC不一致を`E_FILESYSTEM`にすると、利用者はdiskを疑うことになる。
func TestExtractRejectsCorruptContent(t *testing.T) {
	payload := bytes.Repeat([]byte("A"), 64)
	// 宣言sizeは実体と一致させ、CRCだけを壊す。
	var compressed bytes.Buffer
	deflater, deflateErr := flate.NewWriter(&compressed, flate.BestCompression)
	if deflateErr != nil {
		t.Fatalf("flate.NewWriter: %v", deflateErr)
	}
	if _, err := deflater.Write(payload); err != nil {
		t.Fatalf("deflate Write: %v", err)
	}
	if err := deflater.Close(); err != nil {
		t.Fatalf("deflate Close: %v", err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	header := &zip.FileHeader{
		Name:               "broken.bin",
		Method:             zip.Deflate,
		CRC32:              crc32.ChecksumIEEE(payload) ^ 0xFFFF,
		CompressedSize64:   uint64(compressed.Len()),
		UncompressedSize64: uint64(len(payload)),
	}
	header.SetMode(0o644)
	entry, rawErr := writer.CreateRaw(header)
	if rawErr != nil {
		t.Fatalf("CreateRaw: %v", rawErr)
	}
	if _, err := entry.Write(compressed.Bytes()); err != nil {
		t.Fatalf("raw Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}

	harness := newExtractHarness(t, buffer.Bytes())
	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 0, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("CRC不一致のarchiveが通った")
	}
	if err.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
	}
}

// TestExtractRejectsUnsafeEntry は事前検査の違反で1件も書かないことを固定する。
func TestExtractRejectsUnsafeEntry(t *testing.T) {
	archive := buildZip(t, []zipItem{
		{name: "safe.txt", data: []byte("ok"), mode: 0o644},
		{name: "../escape.txt", data: []byte("no"), mode: 0o644},
	})
	harness := newExtractHarness(t, archive)

	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 0, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("traversalを含むarchiveが通った")
	}
	if err.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
	}
	// **1件でも違反があれば展開しない。** 安全なentryだけを書いてはいけない。
	harness.missing(t, "safe.txt")
}

// TestExtractRejectsEntryAppearingAfterInspection は検査後に現れた実体を拒否する。
//
// docs/10-security.md §5「検査と実書込みの間にもparent identity/containmentを
// 確認し、symlink raceを防ぐ」。書込み先にsymlinkを置かれるとWriteStreamは
// link先へ書く。重複は事前検査が拒否済みなので、ここに実体があること自体が異常である。
func TestExtractRejectsEntryAppearingAfterInspection(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	harness.fs.AddLink(payloadDir+"/VERSION", port.LinkSymlink, "/etc/passwd")

	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("書込み先に実体があるまま展開された")
	}
	if err.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
	}
	if !strings.Contains(err.Cause.Error(), "検査後") {
		t.Errorf("cause = %v, want 検査後に現れた実体を示すmessage", err.Cause)
	}
}

// TestExtractRejectsSymlinkedDirectory は展開先directoryがlinkの場合を拒否する。
func TestExtractRejectsSymlinkedDirectory(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	harness.fs.AddLink(payloadDir+"/bin", port.LinkSymlink, "/tmp/elsewhere")

	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("展開先directoryがsymlinkのまま展開された")
	}
	if err.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
	}
}

// TestExtractRejectsDestOutsideStaging は展開先がstagingの外を指す場合を拒否する。
//
// docs/08-install-runtime.md §7手順1「staging payloadの全pathがroot内にある
// ことを再検査する」。文字列としてstaging配下でも、解決すると外を指しうる。
func TestExtractRejectsDestOutsideStaging(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	harness.fs.AddDir("/elsewhere/payload", 0o755)
	harness.fs.AddLink(payloadDir, port.LinkJunction, "/elsewhere/payload")

	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("staging外を指す展開先が通った")
	}
	if err.Code != domain.CodePathUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodePathUnsafe, err.Cause)
	}
}

// TestExtractRemovesDestOnCancel はcancel時に展開先を残さないことを固定する。
//
// docs/08-install-runtime.md §6「中断・失敗・cancel時は
// `tmp/operations/<operation-id>/`をdirectory単位で削除すれば復旧する」。
// 中途半端なpayloadを残すと、次のinstallがそれを完成物と見分けられない。
func TestExtractRemovesDestOnCancel(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 1件目のprogress通知でcancelする。2件目のentryを読み始めた時点で止まる。
	harness.sink.onReport = func() { cancel() }

	_, err := harness.extractor.Extract(
		ctx, harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("cancel後も展開が完了した")
	}
	if err.Code != domain.CodeCancelled {
		t.Fatalf("code = %s, want %s（%v）", err.Code, domain.CodeCancelled, err.Cause)
	}
	for _, rel := range []string{"bin/go", "VERSION", "bin"} {
		harness.missing(t, rel)
	}
	harness.missing(t, "")
}

// TestExtractReportsProgress は展開中のbyte通知を固定する。
func TestExtractReportsProgress(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	if _, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc")); err != nil {
		t.Fatalf("Extract = %v", err.Cause)
	}
	if len(harness.sink.reports) == 0 {
		t.Fatal("progress通知が無い")
	}
	last := harness.sink.reports[len(harness.sink.reports)-1]
	if last.Phase != progress.PhaseExtract || last.Unit != progress.UnitBytes {
		t.Errorf("phase/unit = %s/%s, want %s/%s",
			last.Phase, last.Unit, progress.PhaseExtract, progress.UnitBytes)
	}
	if last.Current != 72 {
		t.Errorf("Current = %d, want 72", last.Current)
	}
	if last.Total == nil || *last.Total != 72 {
		t.Errorf("Total = %v, want 72", last.Total)
	}
}

// swappingFileSystem は2回目以降のOpenで別のarchiveを返す。
//
// tarの2 passの間にarchiveを差し替えられた状態を再現する。
type swappingFileSystem struct {
	*fake.FileSystem
	second []byte
	opens  int
}

func (f *swappingFileSystem) Open(p string) (io.ReadCloser, error) {
	f.opens++
	if f.opens >= 2 && p == archiveFile {
		return io.NopCloser(bytes.NewReader(f.second)), nil
	}
	return f.FileSystem.Open(p)
}

// TestExtractTarGzDetectsSwappedArchive は2 passの間の差し替えを拒否する。
//
// tarはsequential formatで、全entryを知るには2回読むしかない。2 pass目で
// 検査結果と突き合わせないと、1 pass目とは別のarchiveをそのまま展開してしまう。
func TestExtractTarGzDetectsSwappedArchive(t *testing.T) {
	swapped := buildTarGz(t, []tarItem{
		{name: "go/", mode: 0o755, typeflag: tar.TypeDir},
		{name: "go/bin/", mode: 0o755, typeflag: tar.TypeDir},
		{name: "go/bin/go", data: bytes.Repeat([]byte("g"), 64), mode: 0o755, typeflag: tar.TypeReg},
		// 同じ位置のentryをsizeだけ変えた別archive。
		{name: "go/VERSION", data: []byte("go9.99.9-evil"), mode: 0o644, typeflag: tar.TypeReg},
	})
	harness := newExtractHarness(t, goTarGz(t))
	filesystem := &swappingFileSystem{FileSystem: harness.fs, second: swapped}
	extractor, err := NewExtractor(filesystem, progress.NewReporter(harness.sink))
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}

	_, extractErr := extractor.Extract(
		context.Background(), harness.request(t, definition.FormatTarGz, 1, "linux-amd64-glibc"))
	if extractErr == nil {
		t.Fatal("差し替えられたarchiveが通った")
	}
	if extractErr.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）",
			extractErr.Code, domain.CodeArchiveUnsafe, extractErr.Cause)
	}
	if !strings.Contains(extractErr.Cause.Error(), "差し替え") {
		t.Errorf("cause = %v, want 差し替えを示すmessage", extractErr.Cause)
	}
}

// TestExtractRejectsCorruptArchive は読めないarchiveを拒否する。
func TestExtractRejectsCorruptArchive(t *testing.T) {
	tests := []struct {
		name    string
		format  definition.ArchiveFormat
		archive []byte
	}{
		{"zipでない", definition.FormatZip, []byte("not a zip archive at all")},
		{"gzipでない", definition.FormatTarGz, []byte("not a gzip archive at all")},
		{"tarが途中で切れている", definition.FormatTarGz, truncatedTarGz(t)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newExtractHarness(t, test.archive)
			_, err := harness.extractor.Extract(
				context.Background(), harness.request(t, test.format, 0, "linux-amd64-glibc"))
			if err == nil {
				t.Fatal("壊れたarchiveが通った")
			}
			if err.Code != domain.CodeArchiveUnsafe {
				t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
			}
		})
	}
}

// truncatedTarGz はtar streamが途中で終わるtar.gzを作る。
func truncatedTarGz(t *testing.T) []byte {
	t.Helper()
	full := buildTarGz(t, []tarItem{
		{name: "a.txt", data: bytes.Repeat([]byte("a"), 2048), mode: 0o644, typeflag: tar.TypeReg},
	})
	// gzip stream自体は正しく、展開後のtarだけが途中で切れる状態にする。
	reader, err := gzip.NewReader(bytes.NewReader(full))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(raw[:len(raw)-1536]); err != nil {
		t.Fatalf("gzip Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return buffer.Bytes()
}

// TestExtractRejectsInvalidRequest は要求の前提違反を固定する。
func TestExtractRejectsInvalidRequest(t *testing.T) {
	build := func(t *testing.T, mutate func(*ExtractRequest)) ExtractRequest {
		t.Helper()
		harness := newExtractHarness(t, goZip(t))
		req := harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc")
		mutate(&req)
		return req
	}
	pathOf := func(t *testing.T, role domain.PathRole, p string) domain.PathValue {
		t.Helper()
		value, err := domain.NewPathValue(role, p)
		if err != nil {
			t.Fatalf("NewPathValue: %v", err)
		}
		return value
	}

	tests := []struct {
		name   string
		mutate func(*ExtractRequest)
	}{
		{"archive pathが空", func(r *ExtractRequest) { r.ArchivePath = domain.PathValue{} }},
		{"archive roleが違う", func(r *ExtractRequest) {
			r.ArchivePath = pathOf(t, domain.RolePayload, archiveFile)
		}},
		{"staging rootが空", func(r *ExtractRequest) { r.StagingRoot = domain.PathValue{} }},
		{"staging roleが違う", func(r *ExtractRequest) {
			r.StagingRoot = pathOf(t, domain.RoleDataRoot, stagingDir)
		}},
		{"展開先が空", func(r *ExtractRequest) { r.Dest = domain.PathValue{} }},
		{"展開先roleが違う", func(r *ExtractRequest) {
			r.Dest = pathOf(t, domain.RoleStaging, payloadDir)
		}},
		{"hostが未設定", func(r *ExtractRequest) { r.Host = domain.Platform{} }},
		{"stripが負", func(r *ExtractRequest) { r.StripComponents = -1 }},
		{"stripが2", func(r *ExtractRequest) { r.StripComponents = 2 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newExtractHarness(t, goZip(t))
			_, err := harness.extractor.Extract(context.Background(), build(t, test.mutate))
			if err == nil {
				t.Fatal("不正な要求が通った")
			}
			if err.Code != domain.CodeUsage {
				t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeUsage, err.Cause)
			}
		})
	}
}

// TestExtractRejectsUnknownFormat は未知のarchive形式を拒否する。
func TestExtractRejectsUnknownFormat(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	req := harness.request(t, definition.ArchiveFormat("7z"), 1, "linux-amd64-glibc")
	_, err := harness.extractor.Extract(context.Background(), req)
	if err == nil {
		t.Fatal("未知のarchive形式が通った")
	}
	if err.Code != domain.CodeUsage {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeUsage, err.Cause)
	}
}

// TestExtractRejectsNonRegularArchive はarchiveが通常fileでない場合を拒否する。
func TestExtractRejectsNonRegularArchive(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	harness.fs.AddLink(archiveFile, port.LinkSymlink, "/etc/passwd")
	_, err := harness.extractor.Extract(
		context.Background(), harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("symlinkのarchiveが通った")
	}
	if err.Code != domain.CodeArchiveUnsafe {
		t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeArchiveUnsafe, err.Cause)
	}
}

// TestExtractFailureInjection はfilesystem失敗ごとの分類と後始末を固定する。
//
// docs/11-quality-and-ci.md §8のfailure injectionを、展開が触る各操作へ当てる。
// skipは同じ操作の何回目で失敗させるかで、経路ごとに違うcodeへ落ちることを見る。
func TestExtractFailureInjection(t *testing.T) {
	tests := []struct {
		name string
		op   string
		skip int
	}{
		{"archiveのstat", fake.OpStat, 0},
		{"archiveのopen", fake.OpOpenAt, 0},
		{"staging rootの解決", fake.OpRealPath, 0},
		{"展開先の作成", fake.OpMkdirAll, 0},
		{"展開先の解決", fake.OpRealPath, 1},
		{"entry directoryの作成", fake.OpMkdirAll, 1},
		{"entry directoryの確認", fake.OpStat, 1},
		{"entry directoryの解決", fake.OpRealPath, 2},
		{"書込み先の確認", fake.OpStat, 2},
		{"file書込み", fake.OpWriteStream, 0},
		{"書込み後の確認", fake.OpStat, 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newExtractHarness(t, goZip(t))
			harness.injector.Fail(test.op, test.skip, 1, fake.ErrDiskFull)

			_, err := harness.extractor.Extract(
				context.Background(), harness.request(t, definition.FormatZip, 1, "linux-amd64-glibc"))
			if err == nil {
				t.Fatal("失敗注入下で展開が成功した")
			}
			if err.Code != domain.CodeFilesystem {
				t.Errorf("code = %s, want %s（%v）", err.Code, domain.CodeFilesystem, err.Cause)
			}
			// 失敗時に完成物を残さない。
			harness.missing(t, "bin/go")
			harness.missing(t, "VERSION")
			if pending := harness.injector.Pending(); len(pending) != 0 {
				t.Errorf("消化されなかった注入 %v", pending)
			}
		})
	}
}

// TestExtractTarGzFailureInjection はtar経路の読取り失敗を固定する。
func TestExtractTarGzFailureInjection(t *testing.T) {
	for _, skip := range []int{0, 1} {
		harness := newExtractHarness(t, goTarGz(t))
		harness.injector.Fail(fake.OpOpen, skip, 1, fake.ErrDiskFull)

		_, err := harness.extractor.Extract(
			context.Background(), harness.request(t, definition.FormatTarGz, 1, "linux-amd64-glibc"))
		if err == nil {
			t.Fatalf("skip=%d: 失敗注入下で展開が成功した", skip)
		}
		if err.Code != domain.CodeFilesystem {
			t.Errorf("skip=%d: code = %s, want %s（%v）",
				skip, err.Code, domain.CodeFilesystem, err.Cause)
		}
		harness.missing(t, "VERSION")
	}
}

// newTestWriter は上限を指定したtreeWriterを作る。
//
// 実測bytesの上限は`maxExtractBytes`がarchive sizeから決めるため、end-to-endでは
// 境界へ寄せたarchiveを作れない。書込み器を直接組んで境界だけを見る。
func newTestWriter(t *testing.T, harness *extractHarness, maxTotal int64) *treeWriter {
	t.Helper()
	req := harness.request(t, definition.FormatZip, 0, "linux-amd64-glibc")
	harness.fs.AddDir(payloadDir, extractDirPerm)
	return &treeWriter{
		fs:           harness.fs,
		req:          req,
		destReal:     payloadDir,
		maxTotal:     maxTotal,
		verifiedDirs: map[string]struct{}{"": {}},
		reporter:     progress.NewReporter(harness.sink),
	}
}

// TestTreeWriterCutsOffTotalBytes は総展開bytesの上限を固定する。
//
// entryごとに上限内でも、合計で上限を超えるarchiveを通さない。
func TestTreeWriterCutsOffTotalBytes(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	writer := newTestWriter(t, harness, 100)

	first := InspectedEntry{Path: "a.bin", Kind: KindFile, Size: 60}
	if err := writer.writeFile(
		context.Background(), first, bytes.NewReader(bytes.Repeat([]byte("a"), 60))); err != nil {
		t.Fatalf("1件目 = %v", err.Cause)
	}
	second := InspectedEntry{Path: "b.bin", Kind: KindFile, Size: 60}
	err := writer.writeFile(
		context.Background(), second, bytes.NewReader(bytes.Repeat([]byte("b"), 60)))
	if err == nil {
		t.Fatal("総展開bytesの上限を超えた書込みが通った")
	}
	if err.Code != domain.CodeArchiveUnsafe || !errors.Is(err.Cause, errArchiveTooLarge) {
		t.Errorf("err = %s / %v, want %s / errArchiveTooLarge",
			err.Code, err.Cause, domain.CodeArchiveUnsafe)
	}
	harness.missing(t, "b.bin")
}

// TestTreeWriterCutsOffSingleFileLimit は単一fileの上限を固定する。
//
// 宣言sizeが上限以下でも、実bytesが単一file上限を超えれば打ち切る。
func TestTreeWriterCutsOffSingleFileLimit(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	writer := newTestWriter(t, harness, ArchiveTotalMaxBytes)

	// 宣言を単一file上限より大きくし、実bytes側の判定だけを見る。
	item := InspectedEntry{Path: "huge.bin", Kind: KindFile, Size: ArchiveFileMaxBytes + 1}
	reader := &extractReader{
		inner:  bytes.NewReader(bytes.Repeat([]byte("x"), 8)),
		ctx:    context.Background(),
		writer: writer,
		item:   item,
		// 直前まで読んだことにして境界だけを見る。実際に4 GiB読ませない。
		read: ArchiveFileMaxBytes,
	}
	if _, err := io.ReadAll(reader); !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("err = %v, want errArchiveTooLarge", err)
	}
}

// TestExtractReaderStopsOnCancel はcancelがWriteStreamのsrc経由で伝わることを固定する。
//
// WriteStreamはcontextを受け取らない。cancelはsrc側から伝えるしかない。
func TestExtractReaderStopsOnCancel(t *testing.T) {
	harness := newExtractHarness(t, goZip(t))
	writer := newTestWriter(t, harness, ArchiveTotalMaxBytes)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &extractReader{
		inner:  bytes.NewReader([]byte("abc")),
		ctx:    ctx,
		writer: writer,
		item:   InspectedEntry{Path: "a.bin", Kind: KindFile, Size: 3},
	}
	if _, err := reader.Read(make([]byte, 3)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestLimitedReaderStopsAtLimit は展開後streamの上限を固定する。
func TestLimitedReaderStopsAtLimit(t *testing.T) {
	reader := &limitedReader{inner: bytes.NewReader(bytes.Repeat([]byte("x"), 100)), limit: 10}
	if _, err := io.ReadAll(reader); !errors.Is(err, errArchiveTooLarge) {
		t.Fatalf("err = %v, want errArchiveTooLarge", err)
	}
	exact := &limitedReader{inner: bytes.NewReader(bytes.Repeat([]byte("x"), 10)), limit: 10}
	if _, err := io.ReadAll(exact); err != nil {
		t.Fatalf("上限ちょうどが失敗した: %v", err)
	}
}

// TestMaxExtractBytes は実測bytes上限の算出を固定する。
func TestMaxExtractBytes(t *testing.T) {
	tests := []struct {
		name        string
		archiveSize int64
		want        int64
	}{
		{"size不明", 0, ArchiveTotalMaxBytes},
		{"負", -1, ArchiveTotalMaxBytes},
		{"小さいarchiveは圧縮比で決まる", 1000, 1000 * ArchiveRatioMax},
		{
			"境界ちょうど",
			ArchiveTotalMaxBytes / ArchiveRatioMax,
			(ArchiveTotalMaxBytes / ArchiveRatioMax) * ArchiveRatioMax,
		},
		{"総展開上限で頭打ち", ArchiveTotalMaxBytes, ArchiveTotalMaxBytes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := maxExtractBytes(test.archiveSize); got != test.want {
				t.Errorf("maxExtractBytes(%d) = %d, want %d", test.archiveSize, got, test.want)
			}
		})
	}
}

// TestEntryKindMapsFileModes はfile modeから種別への写像を固定する。
func TestEntryKindMapsFileModes(t *testing.T) {
	tests := []struct {
		name          string
		mode          fs.FileMode
		trailingSlash bool
		want          EntryKind
	}{
		{"通常file", 0o644, false, KindFile},
		{"directory", fs.ModeDir | 0o755, false, KindDir},
		{"末尾slashだけ", 0o644, true, KindDir},
		{"symlink", fs.ModeSymlink | 0o777, false, KindSymlink},
		{"device", fs.ModeDevice | 0o644, false, KindOther},
		{"fifo", fs.ModeNamedPipe | 0o644, false, KindOther},
		{"socket", fs.ModeSocket | 0o644, false, KindOther},
		{"irregular", fs.ModeIrregular | 0o644, false, KindOther},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := entryKind(test.mode, test.trailingSlash); got != test.want {
				t.Errorf("entryKind(%v, %v) = %q, want %q",
					test.mode, test.trailingSlash, got, test.want)
			}
		})
	}
}

// TestClampSizeKeepsOverflowAboveLimits はint64へ収まらない宣言sizeを固定する。
//
// 0や負へ丸めると上限検査を素通りする。
func TestClampSizeKeepsOverflowAboveLimits(t *testing.T) {
	if got := clampSize(1 << 63); got <= ArchiveFileMaxBytes {
		t.Errorf("clampSize(1<<63) = %d, 単一file上限を超えていない", got)
	}
	if got := clampSize(42); got != 42 {
		t.Errorf("clampSize(42) = %d, want 42", got)
	}
}

// TestTarEntryMapsTypeflags はtarのtypeflagから種別への写像を固定する。
func TestTarEntryMapsTypeflags(t *testing.T) {
	tests := []struct {
		typeflag byte
		want     EntryKind
	}{
		{tar.TypeReg, KindFile},
		{tar.TypeDir, KindDir},
		{tar.TypeSymlink, KindSymlink},
		{tar.TypeLink, KindHardlink},
		{tar.TypeChar, KindOther},
		{tar.TypeBlock, KindOther},
		{tar.TypeFifo, KindOther},
	}
	for _, test := range tests {
		entry := tarEntry(&tar.Header{Name: "x", Typeflag: test.typeflag, Mode: 0o755})
		if entry.Kind != test.want {
			t.Errorf("typeflag %q = %q, want %q", test.typeflag, entry.Kind, test.want)
		}
		if !entry.Executable {
			t.Errorf("typeflag %q: Executableがfalse", test.typeflag)
		}
		// tarは個別の圧縮後sizeを持たない。
		if entry.CompressedSize != 0 {
			t.Errorf("typeflag %q: CompressedSize = %d, want 0", test.typeflag, entry.CompressedSize)
		}
	}
}

// TestNewExtractorRequiresFileSystem は依存不足を拒否することを固定する。
func TestNewExtractorRequiresFileSystem(t *testing.T) {
	if _, err := NewExtractor(nil, nil); err == nil {
		t.Fatal("FileSystemなしでExtractorが作れた")
	}
}
