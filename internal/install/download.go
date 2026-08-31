package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// ArtifactMaxBytes はartifact downloadの上限である。
//
// docs/04-storage-and-data.md §21「artifact download 20 GiB」。
const ArtifactMaxBytes int64 = 20 << 30

// PartSuffix は書込み途中のdownloadへ付ける拡張子である。
//
// docs/04-storage-and-data.md §17.2の`download-cache` roleが「download cache
// fileまたはその**partial metadata**」を含む。完成fileと途中のfileをbasenameで
// 区別し、cache再利用の判定が途中のfileを拾わないようにする。
const PartSuffix = ".part"

// cacheFilePerm はdownload cache fileのpermissionである。
//
// 所有者だけが読み書きする。docs/10-security.md §6が「group/other書込み不可を
// 基本とする」と定める。
const cacheFilePerm fs.FileMode = 0o600

// Request は1 artifactのdownload要求である。
type Request struct {
	// URL はHTTPSの完全URLである。
	URL string
	// ExpectedDigest はproviderが公開したdigestである（`<algorithm>:<hex>`）。
	//
	// v0.1はchecksumを公開しないartifactを扱わない（docs/10-security.md §7.2）
	// ため、未設定は要求の誤りとして拒否する。
	ExpectedDigest domain.Digest
	// ExpectedSize はproviderが公開したbyte数である。0はunknownを表す。
	ExpectedSize int64
	// CachePath はdownload cache内の最終pathである。
	CachePath domain.PathValue
	// MaxRedirects はredirect追跡の上限である。
	//
	// redirect先hostのallowlist照合は呼出し側（Plan）が行う。定義の
	// `redirect_hosts`はtoolごとの契約であり、downloadはその判断を持たない。
	MaxRedirects int
	// Tool とVersion はprogress通知に載せる。
	Tool    domain.ToolID
	Version domain.Version
	// OperationID はprogress通知に載せるtransaction識別子である。
	OperationID domain.OperationID
}

// Result はdownloadの結果である。
type Result struct {
	// Path は検証済みfileのpathである。
	Path domain.PathValue
	// Size はbyte数である。
	Size int64
	// InternalDigest はgdtvm自身が計算したSHA-256である。
	//
	// receiptとcache identityの照合に使う（docs/04-storage-and-data.md §7）。
	InternalDigest domain.Digest
	// FromCache は既存のcache fileを再利用したかである。
	FromCache bool
}

// Downloader はartifactを取得してdownload cacheへ置く。
//
// docs/02-architecture.md §2「ダウンロード、検証、安全展開、probe、receipt、
// transaction」のうち、download部分である。
type Downloader struct {
	http     port.HTTPClient
	fs       port.FileSystem
	reporter *progress.Reporter
}

// NewDownloader はDownloaderを作る。
//
// `reporter`はnilを許す。progress通知先が無い呼出し（doctorの読取りなど）でも
// downloadそのものは成立する。
func NewDownloader(client port.HTTPClient, filesystem port.FileSystem, reporter *progress.Reporter) (*Downloader, error) {
	if client == nil {
		return nil, errors.New("install: HTTPClientが無い")
	}
	if filesystem == nil {
		return nil, errors.New("install: FileSystemが無い")
	}
	return &Downloader{http: client, fs: filesystem, reporter: reporter}, nil
}

// Download はartifactを取得し、digestを検証してcache pathへ置く。
//
// 手順は`.part`書込み→digest照合→renameである。renameをdigest照合の後に置くのは、
// 未検証のbytesが最終pathへ現れないようにするためである（docs/10-security.md §7.2）。
//
// 既にcacheへ完成fileがあり内部digestが一致する場合は取得しない
// （同§10「download cacheはURL identityとdigestが一致するcomplete fileだけを
// 再利用し、partial fileのRange再開は行わない」）。
func (d *Downloader) Download(ctx context.Context, req Request) (Result, *domain.Error) {
	if err := validateRequest(req); err != nil {
		return Result{}, err
	}

	if result, ok := d.reuseCache(req); ok {
		return result, nil
	}

	partPath := req.CachePath.Path() + PartSuffix
	result, err := d.fetch(ctx, req, partPath)
	if err != nil {
		// 途中まで書けたfileを次回の再開に使わない（docs/15-deferred.md D-24）。
		// WriteStreamの失敗経路でも実装が消すが、digest不一致のように書込みが
		// 成功してから落ちる場合はここで消す。
		_ = d.fs.Remove(partPath)
		return Result{}, err
	}

	if renameErr := d.fs.Rename(partPath, req.CachePath.Path()); renameErr != nil {
		_ = d.fs.Remove(partPath)
		return Result{}, filesystemError(
			fmt.Errorf("install: download cacheへの移動に失敗した: %w", renameErr))
	}
	return result, nil
}

// reuseCache は再利用できるcache fileがあればその結果を返す。
//
// 判定は「完成fileが存在し、内部digestが期待upstream digestから導けること」
// ではなく、**upstream digestそのものの再計算**で行う。内部SHA-256だけを見ると、
// providerがsha512を公開しているartifactでalgorithmの一致を確かめられない。
func (d *Downloader) reuseCache(req Request) (Result, bool) {
	info, err := d.fs.Stat(req.CachePath.Path())
	if err != nil || info.IsDir {
		return Result{}, false
	}
	reader, err := d.fs.Open(req.CachePath.Path())
	if err != nil {
		return Result{}, false
	}
	defer reader.Close()

	hasher, err := security.NewStreamHasher(req.ExpectedDigest.Algorithm(), ArtifactMaxBytes)
	if err != nil {
		return Result{}, false
	}
	if _, err := io.Copy(hasher, reader); err != nil {
		return Result{}, false
	}
	if err := hasher.VerifyUpstream(req.ExpectedDigest); err != nil {
		// 壊れたcacheは黙って使わない。消すのは呼出し側の判断に委ね、ここでは
		// 取得し直す方へ倒す。
		return Result{}, false
	}
	return Result{
		Path:           req.CachePath,
		Size:           hasher.Size(),
		InternalDigest: hasher.Internal(),
		FromCache:      true,
	}, true
}

// fetch はartifactを取得して`.part`へ書き、digestを検証する。
func (d *Downloader) fetch(ctx context.Context, req Request, partPath string) (Result, *domain.Error) {
	response, err := d.http.Get(ctx, port.HTTPRequest{
		URL:          req.URL,
		MaxRedirects: req.MaxRedirects,
		MaxBodyBytes: ArtifactMaxBytes,
	})
	if err != nil {
		return Result{}, networkError(req, err)
	}
	defer response.Body.Close()

	hasher, hashErr := security.NewStreamHasher(req.ExpectedDigest.Algorithm(), ArtifactMaxBytes)
	if hashErr != nil {
		return Result{}, internalError(hashErr)
	}

	// srcをwrapしてprogressとdigestを得る。port境界へ通知や計算を持ち込むと、
	// 同じstreamに対する関心が実装ごとに散る。
	total := req.ExpectedSize
	if total <= 0 && response.ContentLength > 0 {
		total = response.ContentLength
	}
	source := io.TeeReader(&countingReader{
		inner:    response.Body,
		total:    total,
		request:  req,
		reporter: d.reporter,
	}, hasher)

	written, writeErr := d.fs.WriteStream(partPath, cacheFilePerm, source)
	if writeErr != nil {
		if errors.Is(writeErr, security.ErrSizeLimit) {
			return Result{}, checksumError(fmt.Errorf(
				"install: artifactが上限%d byteを超えた", ArtifactMaxBytes))
		}
		if errors.Is(writeErr, context.Canceled) || errors.Is(writeErr, context.DeadlineExceeded) {
			return Result{}, cancelledError(writeErr)
		}
		return Result{}, networkError(req, writeErr)
	}

	// providerが公開したsizeと一致しない応答を通さない。digestが一致しても
	// size宣言と食い違う場合、catalogとPlanの表示が実体と違うことになる。
	if req.ExpectedSize > 0 && written != req.ExpectedSize {
		return Result{}, checksumError(fmt.Errorf(
			"install: sizeが一致しない（期待 %d byte / 実際 %d byte）",
			req.ExpectedSize, written))
	}
	if verifyErr := hasher.VerifyUpstream(req.ExpectedDigest); verifyErr != nil {
		return Result{}, checksumError(verifyErr)
	}

	d.report(req, progress.PhaseVerify, written, &written)
	return Result{
		Path:           req.CachePath,
		Size:           written,
		InternalDigest: hasher.Internal(),
	}, nil
}

// report はprogressを通知する。reporterが無ければ何もしない。
func (d *Downloader) report(req Request, phase progress.Phase, current int64, total *int64) {
	if d.reporter == nil {
		return
	}
	d.reporter.Report(progress.Progress{
		OperationID: req.OperationID,
		Phase:       phase,
		Tool:        req.Tool,
		Version:     req.Version,
		Current:     current,
		Total:       total,
		Unit:        progress.UnitBytes,
	})
}

// countingReader は読んだbyte数をprogressとして通知するreaderである。
type countingReader struct {
	inner    io.Reader
	total    int64
	read     int64
	request  Request
	reporter *progress.Reporter
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		r.read += int64(n)
		if r.reporter != nil {
			var total *int64
			// sizeが分からない配布元では総量を通知しない。0を総量として出すと
			// 進捗率が常に100%か0%になる（§16の`size=0`はunknown）。
			if r.total > 0 {
				value := r.total
				total = &value
			}
			r.reporter.Report(progress.Progress{
				OperationID: r.request.OperationID,
				Phase:       progress.PhaseDownload,
				Tool:        r.request.Tool,
				Version:     r.request.Version,
				Current:     r.read,
				Total:       total,
				Unit:        progress.UnitBytes,
			})
		}
	}
	return n, err
}

// validateRequest は要求の前提を検査する。
func validateRequest(req Request) *domain.Error {
	switch {
	case req.URL == "":
		return usageError(errors.New("install: URLが空"))
	case req.ExpectedDigest.IsZero():
		// v0.1はchecksumを公開しないartifactを扱わない（docs/10-security.md §7.2）。
		return usageError(errors.New("install: 期待するupstream digestが未設定"))
	case req.CachePath.Path() == "":
		return usageError(errors.New("install: cache pathが空"))
	case req.CachePath.Role() != domain.RoleDownloadCache:
		// download成果物はdownload-cache roleへ置く（§17.2）。
		return usageError(fmt.Errorf(
			"install: cache pathのroleが %q（want %q）",
			req.CachePath.Role(), domain.RoleDownloadCache))
	case strings.HasSuffix(req.CachePath.Path(), PartSuffix):
		// 完成fileのpathが`.part`で終わると、cache再利用の判定が途中のfileを拾う。
		return usageError(fmt.Errorf("install: cache pathが %q で終わっている", PartSuffix))
	case req.ExpectedSize < 0:
		return usageError(fmt.Errorf("install: sizeが負（%d）", req.ExpectedSize))
	case req.MaxRedirects < 0:
		return usageError(fmt.Errorf("install: MaxRedirectsが負（%d）", req.MaxRedirects))
	}
	return nil
}

// networkError はnetwork起因の失敗をtyped errorにする。
//
// **cacheが無い状態でnetworkへ到達できない場合を`E_OFFLINE`にする。** 利用者が
// 取るべき行動が違うためで、`E_NETWORK`は再実行で直りうる一時障害、
// `E_OFFLINE`は接続そのものが無い状態を指す。
func networkError(req Request, cause error) *domain.Error {
	code := domain.CodeNetwork
	// offline判定はHTTPClient adapterがport境界で正規化する（[port.ErrOffline]）。
	// syscall errnoをここで見ると規則が2箇所へ散り、fakeがsyscall errorを
	// 作れないためtestでも再現できない。
	if errors.Is(cause, port.ErrOffline) {
		code = domain.CodeOffline
	}
	return &domain.Error{
		Code:      code,
		Retryable: true,
		PathRole:  domain.RoleDownloadCache,
		Cause: fmt.Errorf("install: %s の取得に失敗した: %w",
			security.MaskURL(req.URL), cause),
	}
}

func checksumError(cause error) *domain.Error {
	return &domain.Error{
		Code: domain.CodeChecksumMismatch,
		// 同じbytesを取り直しても同じ結果になる（docs/02-architecture.md §14）。
		Retryable: false,
		PathRole:  domain.RoleDownloadCache,
		Cause:     cause,
	}
}

func filesystemError(cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodeFilesystem,
		Retryable: true,
		PathRole:  domain.RoleDownloadCache,
		Cause:     cause,
	}
}

func cancelledError(cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodeCancelled,
		Retryable: true,
		Cause:     cause,
	}
}

func usageError(cause error) *domain.Error {
	return &domain.Error{Code: domain.CodeUsage, Cause: cause}
}

func internalError(cause error) *domain.Error {
	return domain.Internal(cause)
}
