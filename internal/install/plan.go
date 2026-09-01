package install

import (
	"errors"
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// PlanRequest はinstall Planの組立て入力である（docs/04-storage-and-data.md §16）。
//
// **`Inputs`は呼出し側が供給する。** §16はrevision/digestを「gdtvm自身が計算」と
// 定めるが、計算の主体はそれぞれのfileを所有するpackage（config／registry／
// catalog／store）である。builderが自分で集めると、作成時とExecuteの照合時で読む
// 経路が同じになり、`E_PLAN_STALE`が「同じ関数を2回呼んだ結果の比較」に退化する。
type PlanRequest struct {
	// ClientVersion は§16の`client_version`である。
	ClientVersion string
	// Invocation とOperation は§16のID群である。
	Invocation domain.InvocationID
	Operation  domain.OperationID
	// CreatedAt はPlan作成時刻である。呼出し側がClock portから取る。
	CreatedAt time.Time

	// Tool は対象toolの正規IDである。
	Tool domain.ToolID
	// Item は解決済みcatalog itemである。
	Item store.CatalogItem
	// Platform はdefinitionの当該platform blockである。
	Platform definition.Platform

	// Roots は§12のpath render rootである。
	//
	// `ProbeTemp`はここで設定しない。probeごとに[ProbeTempRoot]配下の別
	// directoryを割り当てるためである。probe以外の文脈で`{{probe_temp}}`が
	// 現れたら[RenderPath]が拒否する。
	Roots RenderRoots
	// ProbeTempRoot はprobe専用temp directoryの親である（role=staging）。
	//
	// docs/06-tool-definition.md §11「**probeごとに**空のowner-only probe tempを
	// 作り、成功/失敗/cancel後にengineが削除する。**probeのcwdはその probe temp
	// とし、呼出し元のcurrent directoryを継承しない**」。probe IDごとに
	// この配下へdirectoryを割り当てる。
	//
	// probeを持つdefinitionでは必須である。渡されなければPlanを作らない。
	ProbeTempRoot domain.PathValue
	// DownloadDestination はartifactの保存先である（role=download-cache|staging）。
	DownloadDestination domain.PathValue
	// StagingDestination は展開先である（role=staging）。
	StagingDestination domain.PathValue

	// Inputs はExecuteが再検査する入力identityである。
	Inputs store.PlanInputs
}

// BuildInstallPlan はinstall operationのPlanを組み立てる（§16）。
//
// 純関数である。外部作用を持たず、同じ入力から同じPlanを返す。時刻とIDは
// 呼出し側がportから取って[PlanRequest]へ載せる。
//
// **definitionの値をstoreのenumへ渡すとき、string castを使わない。**
// 両packageは同じ値集合を別の型で持っており、castだと`definition`側へ値が
// 増えたときに無効なenumを持つPlanを黙って作れてしまう。変換表で受け、
// 未知値はerrorにする。
func BuildInstallPlan(req PlanRequest) (store.Plan, error) {
	if err := req.validate(); err != nil {
		return store.Plan{}, err
	}

	summary, err := buildPlanSummary(req)
	if err != nil {
		return store.Plan{}, err
	}
	download, err := buildPlanDownload(req)
	if err != nil {
		return store.Plan{}, err
	}
	extract, err := buildPlanExtract(req, download.ID)
	if err != nil {
		return store.Plan{}, err
	}
	storage, err := buildPlanStorage(req)
	if err != nil {
		return store.Plan{}, err
	}
	probes, err := buildPlanProbes(req)
	if err != nil {
		return store.Plan{}, err
	}
	warnings, err := buildPlanWarnings(req)
	if err != nil {
		return store.Plan{}, err
	}
	summary.WarningCount = int64(len(warnings))

	return store.Plan{
		ClientVersion: req.ClientVersion,
		Invocation:    req.Invocation,
		Operation:     req.Operation,
		Kind:          store.OperationInstall,
		CreatedAt:     req.CreatedAt,
		Summary:       summary,
		// §8.1「setup/setup-removeのPlanだけは`SetupPlan`を必須とし、他operation
		// ではnullにしてoperation固有fieldをtool summaryやwarning parameterへ
		// 埋め込まない」。
		Setup:     nil,
		Inputs:    req.Inputs,
		Downloads: []store.PlanDownload{download},
		Extracts:  []store.PlanExtract{extract},
		Probes:    probes,
		// §16「`writes[]`は利用者可視の変更だけを列挙する」。install単体では
		// payload、receipt、index、shim、storageはすべてdata root内部であり
		// 列挙しない。current linkとproject fileはselectionの責務であり、
		// `install --use`の選択を反映するのは呼出し側である。
		Writes:   nil,
		Storage:  storage,
		Warnings: warnings,
	}, nil
}

// validate はPlan組立ての前提を確かめる。
func (r PlanRequest) validate() error {
	switch {
	case r.ClientVersion == "":
		return errors.New("install: client versionが未設定")
	case r.Invocation.IsZero():
		return errors.New("install: invocation IDが未設定")
	case r.Operation.IsZero():
		return errors.New("install: operation IDが未設定")
	case r.CreatedAt.IsZero():
		return errors.New("install: 作成時刻が未設定")
	case r.Tool.IsZero():
		return errors.New("install: tool IDが未設定")
	case r.CreatedAt.Location() != time.UTC:
		// §7のtimestampはUTCのRFC3339である。localのままPlanへ載せると、
		// 同じ瞬間が環境ごとに違う文字列になる。
		return errors.New("install: 作成時刻がUTCでない")
	}
	if r.Item.VersionText == "" {
		return errors.New("install: catalog itemのversionが空")
	}
	// §3.1が「導入できないversionをPlanにしない」と定める。ここで落とすのは、
	// 解決層の判定漏れをPlanへ持ち込まないためである。
	if !r.Item.Installable {
		return fmt.Errorf("install: version %s はこのplatformで導入できない", r.Item.VersionText)
	}
	if r.DownloadDestination.IsZero() {
		return errors.New("install: downloadの保存先が未設定")
	}
	switch r.DownloadDestination.Role() {
	case domain.RoleDownloadCache, domain.RoleStaging:
	default:
		// §16「`destination.role=download-cache|staging`」。
		return fmt.Errorf("install: downloadの保存先roleが%sである", r.DownloadDestination.Role())
	}
	if r.StagingDestination.IsZero() {
		return errors.New("install: 展開先が未設定")
	}
	if r.StagingDestination.Role() != domain.RoleStaging {
		// §16「`extracts[].destination.role=staging`」。
		return fmt.Errorf("install: 展開先roleが%sである", r.StagingDestination.Role())
	}
	if r.Roots.Payload.IsZero() {
		return errors.New("install: payload rootが未設定")
	}
	if !r.ProbeTempRoot.IsZero() && r.ProbeTempRoot.Role() != domain.RoleStaging {
		// §11のprobe tempはoperation staging内に置く。別roleを許すと、
		// probeがpayloadやstorageへ書ける経路ができる。
		return fmt.Errorf("install: probe temp rootのroleが%sである", r.ProbeTempRoot.Role())
	}
	if r.Roots.Host.IsZero() {
		return errors.New("install: host platformが未設定")
	}
	return nil
}

// buildPlanSummary は§16の重要要約を作る。
//
// `WarningCount`は呼出し元がwarningを数えてから入れる。§16が「`warning_count`と
// `warnings`の件数を一致させる」と定めており、要約側で先に決めると二重管理になる。
func buildPlanSummary(req PlanRequest) (store.PlanSummary, error) {
	providerKind, err := convertProviderKind(req.Platform.ArtifactKind)
	if err != nil {
		return store.PlanSummary{}, err
	}
	return store.PlanSummary{
		Tool:               req.Tool,
		Version:            req.Item.VersionText,
		Platform:           req.Platform.Platform,
		ProviderKind:       providerKind,
		ProviderName:       req.Platform.Provider.Name,
		ProviderRepository: req.Platform.Provider.Repository,
		ProviderHomepage:   req.Platform.Provider.Homepage,
		ProviderLicense:    req.Platform.Provider.License,
		ProviderRelease:    req.Item.ProviderRelease,
		LicenseNotice:      req.Platform.LicenseNotice.String(),
		Channel:            req.Item.Channel,
		Lifecycle:          req.Item.Lifecycle,
		ExpectedDigest:     req.Item.ArtifactDigest,
		ChecksumSource:     req.Item.ChecksumSource,
	}, nil
}

// buildPlanDownload は§16のdownload 1件を作る。
//
// install operationのartifactはprimary artifact 1件だけである（§7.1）。
func buildPlanDownload(req PlanRequest) (store.PlanDownload, error) {
	providerKind, err := convertProviderKind(req.Platform.ArtifactKind)
	if err != nil {
		return store.PlanDownload{}, err
	}
	if req.Item.ArtifactURL == "" {
		return store.PlanDownload{}, errors.New("install: artifact URLが空")
	}
	if req.Item.ArtifactFile == "" {
		return store.PlanDownload{}, errors.New("install: artifact file名が空")
	}
	if req.Item.ArtifactDigest.IsZero() {
		// docs/10-security.md §8「upstream checksumが公開されているものだけを
		// 採用し、providerが公開したalgorithmでの照合を必須にする」。digestなしの
		// artifactをPlanへ載せると、検証できないまま展開まで進む。
		return store.PlanDownload{}, errors.New("install: artifactのupstream digestが無い")
	}
	return store.PlanDownload{
		ID:                 planDownloadID,
		ProviderKind:       providerKind,
		ProviderName:       req.Platform.Provider.Name,
		ProviderRepository: req.Platform.Provider.Repository,
		ProviderHomepage:   req.Platform.Provider.Homepage,
		ProviderRelease:    req.Item.ProviderRelease,
		URL:                req.Item.ArtifactURL,
		FileName:           req.Item.ArtifactFile,
		Size:               req.Item.ArtifactSize,
		ExpectedDigest:     req.Item.ArtifactDigest,
		ChecksumSource:     req.Item.ChecksumSource,
		License:            req.Platform.Provider.License,
		// §16「officialのadoption reasonだけ空」。
		AdoptionReasonMessage: req.Platform.Provider.AdoptionReason,
		Destination:           req.DownloadDestination,
	}, nil
}

// buildPlanExtract は§16のextract 1件を作る。
func buildPlanExtract(req PlanRequest, downloadID string) (store.PlanExtract, error) {
	format, err := convertArchiveFormat(req.Platform.Artifact.Format)
	if err != nil {
		return store.PlanExtract{}, err
	}
	return store.PlanExtract{
		ID:               planExtractID,
		SourceDownloadID: downloadID,
		Format:           format,
		StripComponents:  int64(req.Platform.Install.StripComponents),
		Destination:      req.StagingDestination,
	}, nil
}

// buildPlanStorage は§16のstorage列を作る。
//
// installでは宣言済みstorageをすべて`action=create`とする。§8のstorageは
// 「導入時に用意する」ものであり、既存を残すか消すかの判断が要るのは
// uninstall側である。
func buildPlanStorage(req PlanRequest) ([]store.PlanStorage, error) {
	if len(req.Platform.Storage) == 0 {
		return nil, nil
	}
	values := make([]store.PlanStorage, 0, len(req.Platform.Storage))
	for _, declared := range req.Platform.Storage {
		root, ok := req.Roots.Storage[declared.ID]
		if !ok {
			return nil, fmt.Errorf("install: storage ID %q のrootが渡されていない", declared.ID)
		}
		kind, err := convertStorageKind(declared.Kind)
		if err != nil {
			return nil, err
		}
		scope, err := convertStorageScope(declared.Scope)
		if err != nil {
			return nil, err
		}
		purge, err := convertStoragePurge(declared.Purge)
		if err != nil {
			return nil, err
		}
		values = append(values, store.PlanStorage{
			ID:     declared.ID,
			Kind:   kind,
			Scope:  scope,
			Target: root,
			Purge:  purge,
			Action: store.StorageCreate,
		})
	}
	return values, nil
}

// buildPlanWarnings は§16.1のwarningを作る。
//
// install operationで生じうるのは4件だけである。残る4件
// （`W_DESTRUCTIVE`／`W_SHELL_MODIFICATION`／`W_MODE_CHANGE`／
// `W_RESTART_REQUIRED`）はuninstall・setupの条件であり、installで立てない。
//
// 承認要否をここで決めない。[store.NewPlanWarning]が§16.1の表から引く。
// codeごとの真偽を作成側に持たせると、同じcodeが場面によって承認要否を変えられる。
func buildPlanWarnings(req PlanRequest) ([]store.PlanWarning, error) {
	var warnings []store.PlanWarning
	var failure error
	add := func(code store.PlanWarningCode, id string, params domain.Parameters) {
		messageID, err := planMessageID(id)
		if err != nil {
			if failure == nil {
				failure = err
			}
			return
		}
		warnings = append(warnings, store.NewPlanWarning(code, messageID, params))
	}
	toolParams := func() domain.Parameters {
		return domain.Parameters{
			"tool":    domain.StringScalar(req.Tool.String()),
			"version": domain.StringScalar(req.Item.VersionText),
		}
	}

	if req.Platform.ArtifactKind == definition.KindThirdParty {
		params := toolParams()
		params["provider"] = domain.StringScalar(req.Platform.Provider.Name)
		params["reason"] = domain.StringScalar(req.Platform.Provider.AdoptionReason)
		add(store.WarnThirdParty, messageThirdParty, params)
	}
	if !req.Platform.LicenseNotice.IsZero() {
		params := toolParams()
		params["notice"] = domain.StringScalar(req.Platform.LicenseNotice.String())
		add(store.WarnRestrictiveLicense, messageRestrictiveLicense, params)
	}
	if req.Item.Channel == domain.ChannelPrerelease {
		add(store.WarnPrerelease, messagePrerelease, toolParams())
	}
	if req.Item.Lifecycle == domain.LifecycleEOL {
		params := toolParams()
		params["evidence"] = domain.StringScalar(req.Item.LifecycleEvidence)
		add(store.WarnEOL, messageEOL, params)
	}
	if failure != nil {
		return nil, failure
	}
	return warnings, nil
}

// install operationで立てうる§16.1 warningのmessage IDである。
const (
	messageThirdParty         = "plan.third_party"
	messageRestrictiveLicense = "plan.restrictive_license"
	messagePrerelease         = "plan.prerelease"
	messageEOL                = "plan.eol"
)

// planDownloadID とplanExtractID は§16のID規則に従う固定IDである。
//
// 「IDはPlan内で種類をまたいで一意なASCII lowercase kebab」。installのdownloadと
// extractは各1件のため、連番を振らず意味のある固定IDにする。
const (
	planDownloadID = "artifact"
	planExtractID  = "artifact-extract"
)
