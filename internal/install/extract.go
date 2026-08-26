package install

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// 展開時のpermission（docs/08-install-runtime.md §6「permissionを正規化し、
// Linux executableのowner executeを保持しsetuid/setgidを除去する」）。
//
// archiveが宣言したmodeを運ばず固定値へ正規化する。setuid/setgid/stickyを
// 落とし忘れる経路をそもそも作らないためである。commit時のread-only化
// （同§7手順5）は別段階であり、展開中はownerが書ける必要がある。
const (
	// extractDirPerm は展開したdirectoryのpermissionである。
	extractDirPerm fs.FileMode = 0o755
	// extractFilePerm は展開した通常fileのpermissionである。
	extractFilePerm fs.FileMode = 0o644
	// extractExecPerm は展開したexecutable fileのpermissionである。
	extractExecPerm fs.FileMode = 0o755
)

// errArchiveTooLarge は展開中に実bytesが上限を超えたことを表す。
//
// sentinelにするのは、[port.FileSystem.WriteStream]を通り抜けたあとで
// 「上限超過」と「読取り失敗」を区別してerror codeを決めるためである。
var errArchiveTooLarge = errors.New("install: 展開中に上限を超えた")

// ExtractRequest は1 archiveの展開要求である。
type ExtractRequest struct {
	// ArchivePath はdigest検証済みのarchiveである（download cache内）。
	ArchivePath domain.PathValue
	// Format はarchive形式である（docs/04-storage-and-data.md §17.1の`zip|tar.gz`）。
	Format definition.ArchiveFormat
	// StagingRoot はoperation stagingのroot directoryである。
	//
	// docs/08-install-runtime.md §6「operation tmpは完成先と同じvolumeへ作り、
	// `tmp/operations/<operation-id>/`配下だけを書く」。展開先がこの配下である
	// ことを、解決後のpathで確認する。
	StagingRoot domain.PathValue
	// Dest は展開先のpayload directoryである。StagingRoot配下を指す。
	//
	// docs/04-storage-and-data.md §17.2はstaging内の展開後内容をrole=payloadと
	// 定める（`staging`の定義が「payloadとして扱う展開後内容を除く」）。
	Dest domain.PathValue
	// StripComponents は先頭から取り除くpath componentの数である（0か1）。
	StripComponents int
	// Host は展開先のplatformである。path規則とcase規則を決める。
	Host domain.Platform
	// Tool、Version、OperationID はprogress通知に載せる。
	Tool        domain.ToolID
	Version     domain.Version
	OperationID domain.OperationID
}

// ExtractResult は展開の結果である。
type ExtractResult struct {
	// Paths は作った実体の相対path（slash区切り）である。宣言順を保つ。
	Paths []string
	// TotalBytes は**実際に書いた**byte数の合計である。宣言値ではない。
	TotalBytes int64
	// FileCount、DirCount は作った実体の数である。
	FileCount int
	DirCount  int
}

// Extractor は検証済みarchiveをstagingへ展開する。
//
// docs/02-architecture.md §2「ダウンロード、検証、安全展開、probe、receipt、
// transaction」のうち、安全展開部分である。
//
// portにしないのは、zip/tar解析が標準libraryの純計算で、書込み・permission・
// 削除がすべて[port.FileSystem]の背後へ閉じているためである（同§4）。展開の
// 安全検査そのものが検証対象であり、portで差し替えられるようにするとその検査を
// testで確かめられなくなる。
type Extractor struct {
	fs       port.FileSystem
	reporter *progress.Reporter
}

// NewExtractor はExtractorを作る。
//
// `reporter`はnilを許す。progress通知先が無い呼出しでも展開そのものは成立する。
func NewExtractor(filesystem port.FileSystem, reporter *progress.Reporter) (*Extractor, error) {
	if filesystem == nil {
		return nil, errors.New("install: FileSystemが無い")
	}
	return &Extractor{fs: filesystem, reporter: reporter}, nil
}

// Extract はarchiveを展開先へ展開する。
//
// 手順は「全entryの事前検査（[InspectEntriesResult]）→ 1 entryずつ書込み」で、
// 書込み中も実bytesの上限、parent identity、containmentを検査し続ける。
// 宣言sizeだけを信じないのは、zip bombが宣言を偽れるためである
// （docs/04-storage-and-data.md §21）。
//
// 失敗・cancel時は展開先をdirectory単位で削除する（docs/08-install-runtime.md
// §6）。中途半端なpayloadを残すと、次のinstallがそれを完成物と見分けられない。
func (e *Extractor) Extract(ctx context.Context, req ExtractRequest) (ExtractResult, *domain.Error) {
	if err := validateExtractRequest(req); err != nil {
		return ExtractResult{}, err
	}
	info, statErr := e.fs.Stat(req.ArchivePath.Path())
	if statErr != nil {
		return ExtractResult{}, extractFilesystemError(
			fmt.Errorf("install: archiveを参照できない: %w", statErr))
	}
	if info.IsDir || info.IsSymlink {
		return ExtractResult{}, archiveUnsafe(errors.New("install: archiveが通常fileでない"))
	}

	result, err := e.run(ctx, req, info.Size)
	if err != nil {
		// RemoveAllはsymlinkを辿らない（port契約）。展開先がlinkへ差し替えられて
		// いてもlink先を消さない。
		_ = e.fs.RemoveAll(req.Dest.Path())
		return ExtractResult{}, err
	}
	return result, nil
}

// run はformatごとの読取りへ振り分ける。
func (e *Extractor) run(ctx context.Context, req ExtractRequest, archiveSize int64) (ExtractResult, *domain.Error) {
	switch req.Format {
	case definition.FormatZip:
		return e.extractZip(ctx, req, archiveSize)
	case definition.FormatTarGz:
		return e.extractTarGz(ctx, req, archiveSize)
	default:
		return ExtractResult{}, usageError(fmt.Errorf(
			"install: archive形式 %q は %s|%s のいずれでもない",
			req.Format, definition.FormatZip, definition.FormatTarGz))
	}
}

// extractZip はzipを展開する。
//
// zipはcentral directoryを1回読めば全entryが得られるため、検査と展開で同じ
// [zip.Reader]を使う。metadataを2回読まないので、その間にarchiveが差し替わる
// 経路が無い。
func (e *Extractor) extractZip(ctx context.Context, req ExtractRequest, archiveSize int64) (ExtractResult, *domain.Error) {
	handle, openErr := e.fs.OpenAt(req.ArchivePath.Path())
	if openErr != nil {
		return ExtractResult{}, extractFilesystemError(
			fmt.Errorf("install: archiveを開けない: %w", openErr))
	}
	defer handle.Close()

	reader, zipErr := zip.NewReader(handle, archiveSize)
	if zipErr != nil {
		return ExtractResult{}, archiveUnsafe(fmt.Errorf("install: zipとして読めない: %w", zipErr))
	}
	if len(reader.File) > ArchiveEntryMax {
		return ExtractResult{}, archiveUnsafe(fmt.Errorf(
			"install: entry数が上限%dを超える（%d件）", ArchiveEntryMax, len(reader.File)))
	}

	entries := make([]Entry, len(reader.File))
	for index, file := range reader.File {
		entries[index] = zipEntry(file)
	}
	inspected, inspectErr := InspectEntriesResult(InspectRequest{
		Entries:         entries,
		StripComponents: req.StripComponents,
		Host:            req.Host,
	})
	if inspectErr != nil {
		return ExtractResult{}, inspectErr
	}

	writer, writerErr := e.newTreeWriter(req, archiveSize, inspected.TotalBytes)
	if writerErr != nil {
		return ExtractResult{}, writerErr
	}
	for _, item := range inspected.Entries {
		if item.Kind == KindDir {
			if err := writer.mkdir(item.Path); err != nil {
				return ExtractResult{}, err
			}
			continue
		}
		if err := writer.writeZipFile(ctx, item, reader.File[item.Index]); err != nil {
			return ExtractResult{}, err
		}
	}
	return writer.result, nil
}

// extractTarGz はtar.gzを展開する。
//
// tarはsequential formatで、全entryを知るには最後まで読む必要がある。§5が
// 「archiveは**展開前に**全entryを検査」と定めるため、header収集と展開で
// 2 passに分ける。**2 pass目では各entryをもう一度検査し、1 pass目の結果と
// 一致することを確認する。** 同じfileを2回開く以上、その間に差し替えられた
// archiveをそのまま展開しないためである。
func (e *Extractor) extractTarGz(ctx context.Context, req ExtractRequest, archiveSize int64) (ExtractResult, *domain.Error) {
	maxTotal := maxExtractBytes(archiveSize)

	entries, scanErr := e.scanTarGz(req, maxTotal)
	if scanErr != nil {
		return ExtractResult{}, scanErr
	}
	inspected, inspectErr := InspectEntriesResult(InspectRequest{
		Entries:         entries,
		StripComponents: req.StripComponents,
		Host:            req.Host,
	})
	if inspectErr != nil {
		return ExtractResult{}, inspectErr
	}

	reader, closer, openErr := e.openTarGz(req, maxTotal)
	if openErr != nil {
		return ExtractResult{}, openErr
	}
	defer closer.Close()

	writer, writerErr := e.newTreeWriter(req, archiveSize, inspected.TotalBytes)
	if writerErr != nil {
		return ExtractResult{}, writerErr
	}

	next := 0
	for index := 0; next < len(inspected.Entries); index++ {
		header, readErr := reader.Next()
		if readErr != nil {
			return ExtractResult{}, tarReadError(readErr)
		}
		item := inspected.Entries[next]
		if index != item.Index {
			// 1 pass目で`strip_components`が落としたtop-level directoryである。
			continue
		}
		next++
		if err := verifyTarItem(header, item, req); err != nil {
			return ExtractResult{}, err
		}
		if item.Kind == KindDir {
			if err := writer.mkdir(item.Path); err != nil {
				return ExtractResult{}, err
			}
			continue
		}
		if err := writer.writeFile(ctx, item, reader); err != nil {
			return ExtractResult{}, err
		}
	}
	return writer.result, nil
}

// scanTarGz は1 pass目としてtarのheaderだけを集める。
func (e *Extractor) scanTarGz(req ExtractRequest, maxTotal int64) ([]Entry, *domain.Error) {
	reader, closer, openErr := e.openTarGz(req, maxTotal)
	if openErr != nil {
		return nil, openErr
	}
	defer closer.Close()

	entries := make([]Entry, 0, 64)
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, tarReadError(readErr)
		}
		// entry数の上限をここでも見る。[InspectEntriesResult]へ渡す前に打ち切らないと、
		// entryを無限に並べたtarでmemoryを使い切る。
		if len(entries) >= ArchiveEntryMax {
			return nil, archiveUnsafe(fmt.Errorf(
				"install: entry数が上限%dを超える", ArchiveEntryMax))
		}
		entries = append(entries, tarEntry(header))
	}
	return entries, nil
}

// openTarGz はtar.gz readerを開く。呼出し側がcloserをCloseする。
//
// 展開後streamへ上限を掛けるのは、entryを書き出す前の`Next`でもgzipが展開を
// 進めるためである。上限が無いと、header間に詰めたgzip bombがfileを1件も
// 作らずにCPUとmemoryを消費できる。tarのblock headerも上限へ数える。
func (e *Extractor) openTarGz(req ExtractRequest, maxTotal int64) (*tar.Reader, io.Closer, *domain.Error) {
	handle, openErr := e.fs.Open(req.ArchivePath.Path())
	if openErr != nil {
		return nil, nil, extractFilesystemError(
			fmt.Errorf("install: archiveを開けない: %w", openErr))
	}
	gzipReader, gzipErr := gzip.NewReader(handle)
	if gzipErr != nil {
		handle.Close()
		return nil, nil, archiveUnsafe(fmt.Errorf("install: gzipとして読めない: %w", gzipErr))
	}
	limited := &limitedReader{inner: gzipReader, limit: maxTotal}
	return tar.NewReader(limited), handle, nil
}

// verifyTarItem は2 pass目のheaderが1 pass目の検査結果と一致することを確認する。
func verifyTarItem(header *tar.Header, item InspectedEntry, req ExtractRequest) *domain.Error {
	entry := tarEntry(header)
	name, checkErr := checkEntry(entry, req.Host.OS() == domain.OSWindows)
	if checkErr != nil {
		return archiveUnsafe(fmt.Errorf("install: %q: %w", header.Name, checkErr))
	}
	stripped, keep, stripErr := stripComponents(name, req.StripComponents)
	if stripErr != nil || !keep {
		return archiveUnsafe(fmt.Errorf(
			"install: entry[%d] %q が1 pass目と一致しない", item.Index, header.Name))
	}
	if stripped != item.Path || entry.Kind != item.Kind ||
		entry.Size != item.Size || entry.Executable != item.Executable {
		return archiveUnsafe(fmt.Errorf(
			"install: entry[%d] %q が検査時と違う（archiveが読取り中に差し替えられた）",
			item.Index, header.Name))
	}
	return nil
}

// newTreeWriter は展開先を用意し、書込み器を作る。
func (e *Extractor) newTreeWriter(req ExtractRequest, archiveSize, declaredTotal int64) (*treeWriter, *domain.Error) {
	stagingReal, realErr := e.fs.RealPath(req.StagingRoot.Path())
	if realErr != nil {
		return nil, extractFilesystemError(
			fmt.Errorf("install: staging rootを解決できない: %w", realErr))
	}
	if mkErr := e.fs.MkdirAll(req.Dest.Path(), extractDirPerm); mkErr != nil {
		return nil, extractFilesystemError(
			fmt.Errorf("install: 展開先を作れない: %w", mkErr))
	}
	destReal, destErr := e.fs.RealPath(req.Dest.Path())
	if destErr != nil {
		return nil, extractFilesystemError(
			fmt.Errorf("install: 展開先を解決できない: %w", destErr))
	}
	// docs/08-install-runtime.md §7手順1「staging payloadの全pathがroot内に
	// あることを再検査する」。起点であるDest自身をここで確認し、以降のentryは
	// 解決後のDestと比べる。
	if !security.IsContained(stagingReal, destReal, req.Host) {
		return nil, pathUnsafeError(errors.New(
			"install: 展開先がoperation stagingの外を指す"))
	}
	return &treeWriter{
		fs:            e.fs,
		req:           req,
		destReal:      destReal,
		maxTotal:      maxExtractBytes(archiveSize),
		declaredTotal: declaredTotal,
		verifiedDirs:  map[string]struct{}{"": {}},
		reporter:      e.reporter,
	}, nil
}

// treeWriter は検査済みentryをstagingへ書く。
//
// 実bytesの上限、cancel、parent identity/containmentの再検査をここへ集める。
type treeWriter struct {
	fs  port.FileSystem
	req ExtractRequest
	// destReal は展開先をrealpathへ解決した結果である。以降に作る全pathの
	// containmentをこれと比べる。
	destReal string
	// maxTotal は**実測**bytesの上限である。宣言sizeに対する上限とは別に効く。
	maxTotal int64
	// declaredTotal はprogress通知の総量である（宣言値）。
	declaredTotal int64
	// written はここまでに実際に書いたbyte数である。
	written int64
	// verifiedDirs はparent identity/containmentを確認済みの相対pathである。
	// 展開先自身（空文字列）は[Extractor.newTreeWriter]が確認済みとして入れる。
	verifiedDirs map[string]struct{}
	result       ExtractResult
	reporter     *progress.Reporter
}

// mkdir はdirectory entryを作る。
func (w *treeWriter) mkdir(rel string) *domain.Error {
	if _, err := w.ensureDir(rel); err != nil {
		return err
	}
	w.result.Paths = append(w.result.Paths, rel)
	w.result.DirCount++
	return nil
}

// writeZipFile はzip entryを展開する。
func (w *treeWriter) writeZipFile(ctx context.Context, item InspectedEntry, file *zip.File) *domain.Error {
	source, openErr := file.Open()
	if openErr != nil {
		return archiveUnsafe(fmt.Errorf("install: %q を展開できない: %w", item.Path, openErr))
	}
	defer source.Close()
	return w.writeFile(ctx, item, source)
}

// writeFile は1 fileを書く。
//
// docs/10-security.md §5「検査と実書込みの間にもparent identity/containmentを
// 確認し、symlink raceを防ぐ」。事前検査はarchiveが宣言したnameに対するもので、
// 展開先のfilesystemが検査後に差し替えられる可能性はそれでは塞げない。
func (w *treeWriter) writeFile(ctx context.Context, item InspectedEntry, source io.Reader) *domain.Error {
	parent := path.Dir(item.Path)
	if parent == "." {
		parent = ""
	}
	if _, err := w.ensureDir(parent); err != nil {
		return err
	}
	absolute, joinErr := w.absolutePath(item.Path)
	if joinErr != nil {
		return joinErr
	}

	// 書込み先に何も無いことを確認する。重複は事前検査が拒否済みなので、ここに
	// 実体があるなら展開中に外から置かれたものである。symlinkだった場合、
	// WriteStreamはlink先へ書いてしまう。
	if _, statErr := w.fs.Stat(absolute); statErr == nil {
		return archiveUnsafe(fmt.Errorf(
			"install: 展開先 %q に検査後へ現れた実体がある", item.Path))
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return extractFilesystemError(
			fmt.Errorf("install: 展開先 %q を確認できない: %w", item.Path, statErr))
	}

	perm := extractFilePerm
	if item.Executable {
		// docs/08-install-runtime.md §6「Linux executableのowner executeを保持」。
		perm = extractExecPerm
	}
	reader := &extractReader{inner: source, ctx: ctx, writer: w, item: item}
	written, writeErr := w.fs.WriteStream(absolute, perm, reader)
	if writeErr != nil {
		// WriteStreamは失敗時に書きかけを消す（port契約）。ここでは分類だけ行う。
		switch {
		case errors.Is(writeErr, context.Canceled), errors.Is(writeErr, context.DeadlineExceeded):
			return cancelledError(writeErr)
		case errors.Is(writeErr, errArchiveTooLarge):
			return archiveUnsafe(writeErr)
		case isArchiveReadError(writeErr):
			return archiveUnsafe(
				fmt.Errorf("install: %q を展開できない: %w", item.Path, writeErr))
		}
		return extractFilesystemError(
			fmt.Errorf("install: %q を書けない: %w", item.Path, writeErr))
	}
	// 宣言sizeと実bytesが食い違うarchiveを通さない。事前検査が見た総量が実体と
	// 無関係になり、上限判定の意味が無くなる。
	if written != item.Size {
		return archiveUnsafe(fmt.Errorf(
			"install: %q の展開後sizeが宣言と違う（宣言 %d byte / 実際 %d byte）",
			item.Path, item.Size, written))
	}
	if err := w.verifyWritten(absolute, item, written); err != nil {
		return err
	}

	w.written += written
	w.result.TotalBytes += written
	w.result.FileCount++
	w.result.Paths = append(w.result.Paths, item.Path)
	w.report(w.written)
	return nil
}

// verifyWritten は書いた先が想定どおりの実体かを確認する。
func (w *treeWriter) verifyWritten(absolute string, item InspectedEntry, written int64) *domain.Error {
	info, statErr := w.fs.Stat(absolute)
	if statErr != nil {
		return extractFilesystemError(
			fmt.Errorf("install: %q を書込み後に確認できない: %w", item.Path, statErr))
	}
	if info.IsSymlink || info.IsDir {
		return archiveUnsafe(fmt.Errorf(
			"install: %q が書込み後に通常fileでなくなっている", item.Path))
	}
	if info.Size != written {
		return archiveUnsafe(fmt.Errorf(
			"install: %q の書込み後sizeが違う（書込み %d byte / 実体 %d byte）",
			item.Path, written, info.Size))
	}
	return nil
}

// ensureDir は相対pathのdirectoryを作り、identityとcontainmentを確認する。
//
// 一度確認したdirectoryは記録して再確認しない。同じdirectoryへ数万fileを書く
// archiveで、file 1件ごとにRealPathを呼ぶとsyscallが実bytesの書込みを上回る。
func (w *treeWriter) ensureDir(rel string) (string, *domain.Error) {
	absolute, joinErr := w.absolutePath(rel)
	if joinErr != nil {
		return "", joinErr
	}
	if _, done := w.verifiedDirs[rel]; done {
		return absolute, nil
	}
	if mkErr := w.fs.MkdirAll(absolute, extractDirPerm); mkErr != nil {
		return "", extractFilesystemError(
			fmt.Errorf("install: directory %q を作れない: %w", rel, mkErr))
	}

	info, statErr := w.fs.Stat(absolute)
	if statErr != nil {
		return "", extractFilesystemError(
			fmt.Errorf("install: directory %q を確認できない: %w", rel, statErr))
	}
	if info.IsSymlink {
		// linkのままcontainmentを通しても、書込みはlink先へ行く。
		return "", archiveUnsafe(fmt.Errorf("install: 展開先 %q がsymlinkである", rel))
	}
	if !info.IsDir {
		return "", archiveUnsafe(fmt.Errorf("install: 展開先 %q がdirectoryでない", rel))
	}
	resolved, realErr := w.fs.RealPath(absolute)
	if realErr != nil {
		return "", extractFilesystemError(
			fmt.Errorf("install: directory %q を解決できない: %w", rel, realErr))
	}
	if !security.IsContained(w.destReal, resolved, w.req.Host) {
		return "", pathUnsafeError(fmt.Errorf(
			"install: 展開先 %q が解決後にpayload rootの外を指す", rel))
	}
	w.verifiedDirs[rel] = struct{}{}
	return absolute, nil
}

// absolutePath は展開先rootからentryのabsolute pathを組み立てる。
//
// 文字列連結ではなく[security.Join]へcomponent列を渡す。componentのまま渡せば
// `..`や区切り混在が検出でき、`filepath.Join`が先に潰した`..`を見逃さない。
func (w *treeWriter) absolutePath(rel string) (string, *domain.Error) {
	var components []string
	if rel != "" {
		components = strings.Split(rel, "/")
	}
	value, err := security.Join(security.JoinRequest{
		Root:       w.req.Dest,
		Components: components,
		Host:       w.req.Host,
	})
	if err != nil {
		return "", pathUnsafeError(fmt.Errorf("install: %q: %w", rel, err))
	}
	return value.Path(), nil
}

// report はprogressを通知する。reporterが無ければ何もしない。
func (w *treeWriter) report(current int64) {
	if w.reporter == nil {
		return
	}
	var total *int64
	// 総量が分からない場合は通知しない。0を総量として出すと進捗率が常に
	// 100%か0%になる（docs/04-storage-and-data.md §16の`size=0`はunknown）。
	if w.declaredTotal > 0 {
		value := w.declaredTotal
		total = &value
	}
	w.reporter.Report(progress.Progress{
		OperationID: w.req.OperationID,
		Phase:       progress.PhaseExtract,
		Tool:        w.req.Tool,
		Version:     w.req.Version,
		Current:     current,
		Total:       total,
		Unit:        progress.UnitBytes,
	})
}

// extractReader は展開中の1 entryを読むreaderである。
//
// 宣言sizeを信じず**実bytes**で打ち切る。zip bombは宣言を偽れるため、事前検査の
// 宣言sizeだけでは総展開量を抑えられない（docs/04-storage-and-data.md §21）。
type extractReader struct {
	inner io.Reader
	// ctx はcancelの伝播経路である。WriteStreamはcontextを受け取らないため、
	// cancelはsrc側から伝える。
	ctx    context.Context
	writer *treeWriter
	item   InspectedEntry
	read   int64
}

func (r *extractReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.inner.Read(p)
	if n <= 0 {
		return n, err
	}
	r.read += int64(n)
	switch {
	case r.read > r.item.Size:
		return n, fmt.Errorf("%w: %q が宣言 %d byteを超えた", errArchiveTooLarge, r.item.Path, r.item.Size)
	case r.read > ArchiveFileMaxBytes:
		return n, fmt.Errorf("%w: %q が単一file上限 %d byteを超えた",
			errArchiveTooLarge, r.item.Path, ArchiveFileMaxBytes)
	case r.writer.written+r.read > r.writer.maxTotal:
		return n, fmt.Errorf("%w: 総展開bytesが上限 %d byteを超えた",
			errArchiveTooLarge, r.writer.maxTotal)
	}
	r.writer.report(r.writer.written + r.read)
	return n, err
}

// limitedReader は展開後streamに上限を掛ける。
//
// [security.StreamHasher]と違いdigestを持たない。tarのheader走査のように、
// 内容を保存せず展開量だけを抑えたい経路で使う。
type limitedReader struct {
	inner io.Reader
	limit int64
	read  int64
}

func (r *limitedReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.read > r.limit {
			return n, fmt.Errorf("%w: 展開後streamが上限 %d byteを超えた", errArchiveTooLarge, r.limit)
		}
	}
	return n, err
}

// maxExtractBytes は実展開bytesの上限を返す。
//
// docs/04-storage-and-data.md §21の総展開20 GiBと圧縮比1,000の両方で抑える。
// tarのentryは個別の圧縮後sizeを持たず事前検査で比を出せないため、実測bytesに
// 対して効く分母はarchive file自身のsizeだけである。header込みで分母が
// 大きくなる分だけ緩いが、緩い側へ倒れるので通すべきarchiveを拒否しない。
func maxExtractBytes(archiveSize int64) int64 {
	if archiveSize <= 0 {
		return ArchiveTotalMaxBytes
	}
	// 先に割ってからの比較で乗算overflowを避ける。
	if archiveSize > ArchiveTotalMaxBytes/ArchiveRatioMax {
		return ArchiveTotalMaxBytes
	}
	return archiveSize * ArchiveRatioMax
}

// zipEntry はzipのfile headerを[Entry]へ写す。
func zipEntry(file *zip.File) Entry {
	mode := file.Mode()
	return Entry{
		Name: file.Name,
		// Windowsで作られたzipはUnix modeを持たないため、directoryは末尾の
		// `/`でしか判別できない。
		Kind:           entryKind(mode, strings.HasSuffix(file.Name, "/")),
		Size:           clampSize(file.UncompressedSize64),
		CompressedSize: clampSize(file.CompressedSize64),
		Executable:     mode.Perm()&0o100 != 0,
	}
}

// tarEntry はtarのheaderを[Entry]へ写す。
//
// CompressedSizeは0（不明）とする。tarは個別の圧縮後sizeを持たないためで、
// 圧縮比はarchive全体と実測bytesで見る。
func tarEntry(header *tar.Header) Entry {
	entry := Entry{
		Name:       header.Name,
		Size:       header.Size,
		Executable: header.FileInfo().Mode().Perm()&0o100 != 0,
	}
	switch header.Typeflag {
	case tar.TypeReg:
		entry.Kind = KindFile
	case tar.TypeDir:
		entry.Kind = KindDir
	case tar.TypeSymlink:
		entry.Kind = KindSymlink
	case tar.TypeLink:
		entry.Kind = KindHardlink
	default:
		entry.Kind = KindOther
	}
	return entry
}

// entryKind はfile modeから[EntryKind]を決める。
func entryKind(mode fs.FileMode, trailingSlash bool) EntryKind {
	switch {
	case mode&fs.ModeSymlink != 0:
		return KindSymlink
	case mode.IsDir() || trailingSlash:
		return KindDir
	case mode&fs.ModeType != 0:
		// device、fifo、socket、irregular。
		return KindOther
	default:
		return KindFile
	}
}

// clampSize はzipのuint64 sizeをint64へ写す。
//
// int64へ収まらない宣言はMaxInt64にする。0や負へ丸めると上限検査を素通りする。
func clampSize(size uint64) int64 {
	if size > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(size)
}

// isArchiveReadError はarchive読取り由来の失敗かを判定する。
//
// [port.FileSystem.WriteStream]はsrc（archiveのdecompressor）が返したerrorを
// そのまま返す。filesystemの失敗と区別しないと、CRC不一致や壊れたheaderが
// `E_FILESYSTEM`になり、利用者はdiskを疑うことになる。
func isArchiveReadError(err error) bool {
	return errors.Is(err, zip.ErrChecksum) ||
		errors.Is(err, zip.ErrFormat) ||
		errors.Is(err, tar.ErrHeader) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// tarReadError はtar読取り失敗をtyped errorにする。
//
// 途中で切れたarchiveも上限超過も、そのarchiveを展開できないという同じ結論に
// なるため`E_ARCHIVE_UNSAFE`とする。
func tarReadError(cause error) *domain.Error {
	if errors.Is(cause, io.ErrUnexpectedEOF) || errors.Is(cause, io.EOF) {
		return archiveUnsafe(fmt.Errorf("install: archiveが途中で終わっている: %w", cause))
	}
	return archiveUnsafe(fmt.Errorf("install: tarとして読めない: %w", cause))
}

// validateExtractRequest は要求の前提を検査する。
func validateExtractRequest(req ExtractRequest) *domain.Error {
	switch {
	case req.ArchivePath.Path() == "":
		return usageError(errors.New("install: archive pathが空"))
	case req.ArchivePath.Role() != domain.RoleDownloadCache:
		// 展開するのは検証済みcache内のartifactだけである（§17.2）。
		return usageError(fmt.Errorf(
			"install: archive pathのroleが %q（want %q）",
			req.ArchivePath.Role(), domain.RoleDownloadCache))
	case req.StagingRoot.Path() == "":
		return usageError(errors.New("install: staging rootが空"))
	case req.StagingRoot.Role() != domain.RoleStaging:
		return usageError(fmt.Errorf(
			"install: staging rootのroleが %q（want %q）",
			req.StagingRoot.Role(), domain.RoleStaging))
	case req.Dest.Path() == "":
		return usageError(errors.New("install: 展開先が空"))
	case req.Dest.Role() != domain.RolePayload:
		// staging内の展開後内容はpayloadとして扱う（§17.2の`staging`の定義）。
		return usageError(fmt.Errorf(
			"install: 展開先のroleが %q（want %q）", req.Dest.Role(), domain.RolePayload))
	case req.Host.IsZero():
		return usageError(errors.New("install: host platformが未設定"))
	case req.StripComponents < 0 || req.StripComponents > 1:
		return usageError(fmt.Errorf(
			"install: strip_componentsは0か1だけ（%d）", req.StripComponents))
	}
	return nil
}

// extractFilesystemError は展開中のfilesystem失敗をtyped errorにする。
func extractFilesystemError(cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodeFilesystem,
		Retryable: true,
		PathRole:  domain.RolePayload,
		Cause:     cause,
	}
}

// pathUnsafeError は封じ込め違反をtyped errorにする。
//
// archiveの内容ではなく展開先の状態が原因のため、`E_ARCHIVE_UNSAFE`と分ける。
func pathUnsafeError(cause error) *domain.Error {
	return &domain.Error{
		Code: domain.CodePathUnsafe,
		// 同じ状態で再実行しても同じ結果になる（docs/02-architecture.md §14）。
		Retryable: false,
		PathRole:  domain.RolePayload,
		Cause:     cause,
	}
}
