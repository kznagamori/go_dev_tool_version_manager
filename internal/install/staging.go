package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// StageRequest はPlanをstagingまで実行するための入力である。
//
// docs/08-install-runtime.md §2の`downloading → verifying → staging`に対応する。
// `validating`以降はcommit側の責務である。
type StageRequest struct {
	// Plan は承認済みのPlanである。
	//
	// **Plan外のdownload/extractを行わない**（docs/02-architecture.md §8手順5）。
	// 実行する作用はこのPlanの列挙がすべてである。
	Plan store.Plan
	// OperationsRoot は`tmp/operations/`である（role=staging）。
	OperationsRoot domain.PathValue
	// Host はpath規則を決めるplatformである。
	Host domain.Platform
	// Version はprogress通知へ載せる解決済みversionである。
	//
	// **Planから復元しない。** `summary.version`は正規化済みの文字列だが
	// version schemeを持たないため、[domain.Version]へ戻すにはtoolのschemeが要る。
	// 解決したcatalog itemを持つ呼出し側から受け取る（P6-02で確立した
	// 「集めずに受け取る」形）。
	Version domain.Version
	// MaxRedirects はdownloadのredirect追跡上限である。
	MaxRedirects int
}

// StageResult はstagingまで終えた結果である。
type StageResult struct {
	// OperationDir は`tmp/operations/<operation-id>/`である。
	//
	// 呼出し側はcommit後、または失敗時にこのdirectoryごと削除する。
	OperationDir domain.PathValue
	// PayloadDir は展開済みpayloadのrootである（role=payload）。
	PayloadDir domain.PathValue
	// Downloads はdownload IDごとの結果である。
	Downloads map[string]Result
	// Extracts はextract IDごとの結果である。
	Extracts map[string]ExtractResult
}

// Stager はPlanのdownloadとextractをoperation staging内で実行する。
//
// docs/02-architecture.md §4「効果がすべて既存portの背後へ閉じているorchestration
// はportにしない」。ここはDownloader/Extractorと[port.FileSystem]の呼び分けだけを
// 行い、自分では外部作用を持たない。
type Stager struct {
	fs         port.FileSystem
	downloader *Downloader
	extractor  *Extractor
	reporter   *progress.Reporter
}

// NewStager はStagerを作る。
func NewStager(
	filesystem port.FileSystem, downloader *Downloader, extractor *Extractor,
	reporter *progress.Reporter,
) (*Stager, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("install: FileSystem portが未設定")
	case downloader == nil:
		return nil, errors.New("install: Downloaderが未設定")
	case extractor == nil:
		return nil, errors.New("install: Extractorが未設定")
	}
	return &Stager{
		fs: filesystem, downloader: downloader, extractor: extractor,
		reporter: reporter,
	}, nil
}

// Stage はPlanのdownloadとextractを実行する。
//
// docs/08-install-runtime.md §6「operation tmpは完成先と同じvolumeへ作り、
// `tmp/operations/<operation-id>/`配下だけを書く。payload/storage/currentへ
// 直接書かない」。
//
// **失敗しても後始末をここで行わない。** §6が「中断・失敗・cancel時は
// `tmp/operations/<operation-id>/`をdirectory単位で削除すれば復旧する」と定める
// とおり、後始末はdirectoryごとの削除1回で足りる。途中で部分削除すると、
// 何が残っているかが失敗経路ごとに変わって復旧手順が増える。呼出し側が
// [Stager.Cleanup]を1回呼ぶ形にしている。
func (s *Stager) Stage(ctx context.Context, req StageRequest) (StageResult, *domain.Error) {
	if err := req.validate(); err != nil {
		return StageResult{}, domain.Internal(err)
	}
	operationDir, err := s.operationDir(req)
	if err != nil {
		return StageResult{}, err
	}
	if mkErr := s.fs.MkdirAll(operationDir.Path(), stagingDirPerm); mkErr != nil {
		return StageResult{}, domain.Internal(fmt.Errorf(
			"install: operation directoryを作れない: %w", mkErr))
	}

	result := StageResult{
		OperationDir: operationDir,
		Downloads:    make(map[string]Result, len(req.Plan.Downloads)),
		Extracts:     make(map[string]ExtractResult, len(req.Plan.Extracts)),
	}
	for _, download := range req.Plan.Downloads {
		// §2「cancelはdownload、checksum取得、展開entry、probeの境界で確認する」。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, cancelledError(ctxErr)
		}
		downloaded, downloadErr := s.downloader.Download(ctx, Request{
			URL:            download.URL,
			ExpectedDigest: download.ExpectedDigest,
			ExpectedSize:   download.Size,
			CachePath:      download.Destination,
			MaxRedirects:   req.MaxRedirects,
			Tool:           req.Plan.Summary.Tool,
			Version:        req.Version,
			OperationID:    req.Plan.Operation,
		})
		if downloadErr != nil {
			return result, downloadErr
		}
		result.Downloads[download.ID] = downloaded
	}

	payloadDir, err := s.payloadDir(operationDir, req.Host)
	if err != nil {
		return result, err
	}
	result.PayloadDir = payloadDir

	for _, extract := range req.Plan.Extracts {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, cancelledError(ctxErr)
		}
		source, ok := result.Downloads[extract.SourceDownloadID]
		if !ok {
			// §16は`source_download_id`を同じPlanのdownload IDと定める。
			// codecが参照整合性を検査済みだが、ここでも確かめる——Planを
			// 組み立てる経路が増えたときに、検査を通らない値で到達しうる。
			return result, domain.Internal(fmt.Errorf(
				"install: extract %q のsource download %q がPlanに無い",
				extract.ID, extract.SourceDownloadID))
		}
		format, convertErr := planArchiveFormat(extract.Format)
		if convertErr != nil {
			return result, domain.Internal(convertErr)
		}
		extracted, extractErr := s.extractor.Extract(ctx, ExtractRequest{
			ArchivePath:     source.Path,
			Format:          format,
			StagingRoot:     operationDir,
			Dest:            payloadDir,
			StripComponents: int(extract.StripComponents),
			Host:            req.Host,
			Tool:            req.Plan.Summary.Tool,
			Version:         req.Version,
			OperationID:     req.Plan.Operation,
		})
		if extractErr != nil {
			return result, extractErr
		}
		result.Extracts[extract.ID] = extracted
	}
	return result, nil
}

// Cleanup はoperation directoryをまとめて削除する。
//
// docs/04-storage-and-data.md §2「`tmp/operations/<operation-id>/`配下には当該
// operationが作成したものしか存在しない。中断した操作の後始末は、root ID・owner・
// 作成時刻を検査したうえでこのdirectoryをまとめて削除する」。
//
// **この関数が扱うのは自分のoperationのdirectoryだけである。** root ID・owner・
// 作成時刻の検査が要るのは、**他のoperationが残したdirectory**を掃除する場合で
// ある（起動時のcleanup）。自分が今作ったdirectoryは所有が自明なため、
// operation IDが一致することだけを確かめる。
//
// docs/08-install-runtime.md §2「commit後の一時file清掃失敗は成功＋
// `W_CLEANUP_INCOMPLETE`とし、正常payloadを巻き戻さない」。したがって失敗を
// errorとして返しはするが、呼出し側はcommit後ならwarningへ変換する。
func (s *Stager) Cleanup(operationDir domain.PathValue, operation domain.OperationID) error {
	if operationDir.IsZero() || operationDir.Path() == "" {
		return errors.New("install: operation directoryが未設定")
	}
	if operationDir.Role() != domain.RoleStaging {
		return fmt.Errorf("install: operation directoryのroleが%sである", operationDir.Role())
	}
	// 削除対象がこのoperationのものであることをpathの末尾componentで確かめる。
	// 別のoperationのdirectoryを渡されても消さないための最後の歯止めである。
	if !hasOperationSuffix(operationDir.Path(), operation) {
		return fmt.Errorf(
			"install: operation directoryがoperation %s のものでない", operation)
	}
	if err := s.fs.RemoveAll(operationDir.Path()); err != nil {
		return fmt.Errorf("install: operation directoryを削除できない: %w", err)
	}
	return nil
}

// validate はstaging要求の前提を確かめる。
func (r StageRequest) validate() error {
	switch {
	case r.Plan.Kind != store.OperationInstall:
		return fmt.Errorf("install: operationが%sである（installだけを扱う）", r.Plan.Kind)
	case r.Plan.Operation.IsZero():
		return errors.New("install: operation IDが未設定")
	case r.OperationsRoot.IsZero():
		return errors.New("install: operations rootが未設定")
	case r.OperationsRoot.Role() != domain.RoleStaging:
		return fmt.Errorf("install: operations rootのroleが%sである", r.OperationsRoot.Role())
	case r.Host.IsZero():
		return errors.New("install: host platformが未設定")
	case len(r.Plan.Downloads) == 0:
		return errors.New("install: Planにdownloadが無い")
	}
	return nil
}

// operationDir は`tmp/operations/<operation-id>/`を組み立てる。
func (s *Stager) operationDir(req StageRequest) (domain.PathValue, *domain.Error) {
	value, err := security.Join(security.JoinRequest{
		Root:       req.OperationsRoot,
		Components: []string{req.Plan.Operation.String()},
		Host:       req.Host,
	})
	if err != nil {
		return domain.PathValue{}, domain.Internal(fmt.Errorf(
			"install: operation directoryを組み立てられない: %w", err))
	}
	return value, nil
}

// payloadDir はoperation staging内の展開先を返す。
//
// docs/04-storage-and-data.md §17.2はstaging内の展開後内容をrole=payloadと
// 定めるため、roleを付け替える。
func (s *Stager) payloadDir(
	operationDir domain.PathValue, host domain.Platform,
) (domain.PathValue, *domain.Error) {
	joined, err := security.Join(security.JoinRequest{
		Root: operationDir, Components: []string{payloadComponent}, Host: host,
	})
	if err != nil {
		return domain.PathValue{}, domain.Internal(fmt.Errorf(
			"install: payload directoryを組み立てられない: %w", err))
	}
	value, roleErr := domain.NewPathValue(domain.RolePayload, joined.Path())
	if roleErr != nil {
		return domain.PathValue{}, domain.Internal(fmt.Errorf(
			"install: payload directoryのroleを付けられない: %w", roleErr))
	}
	return value, nil
}

// planArchiveFormat はPlanのarchive formatをdefinition側の型へ戻す。
//
// [Extractor]がdefinitionの型を取るためである。P6-02の変換表と同じ理由で
// string castを使わない——片方へ値が増えたときに、もう片方が知らない値を
// 黙って通してしまう。
func planArchiveFormat(format store.ArchiveFormat) (definition.ArchiveFormat, error) {
	for source, target := range archiveFormats {
		if target == format {
			return source, nil
		}
	}
	return "", fmt.Errorf("install: 未知のarchive format %q", format)
}

// hasOperationSuffix はpathの末尾componentがoperation IDかを返す。
func hasOperationSuffix(path string, operation domain.OperationID) bool {
	id := operation.String()
	if id == "" || len(path) <= len(id) {
		return false
	}
	if path[len(path)-len(id):] != id {
		return false
	}
	// 直前がpath区切りであることまで見る。`...xabc`が`abc`で終わることを
	// 一致と誤判定しないためである。
	separator := path[len(path)-len(id)-1]
	return separator == '/' || separator == '\\'
}

const (
	// payloadComponent はoperation staging内の展開先component名である。
	payloadComponent = "payload"
	// stagingDirPerm はoperation directoryのpermissionである。
	//
	// docs/08-install-runtime.md §6「permissionを正規化し」に従い、
	// 展開先（[extractDirPerm]）と同じ値を使う。
	stagingDirPerm fs.FileMode = 0o755
)
