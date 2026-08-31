package install

import (
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// testMessageID は検査で使う任意のmessage IDを返す。
func testMessageID(t *testing.T) domain.MessageID {
	t.Helper()
	id, err := domain.ParseMessageID("plan.probe_reason")
	if err != nil {
		t.Fatalf("ParseMessageID: %v", err)
	}
	return id
}

// testDigest は検査で使うupstream digestを返す。
//
// **実在のartifactのdigestではない。** 検査は値の運搬だけを見るため、
// 固定の16進文字列で足りる。
func testDigest(t *testing.T) domain.Digest {
	t.Helper()
	digest, err := domain.ParseUpstreamDigest("sha256:" + strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("ParseUpstreamDigest: %v", err)
	}
	return digest
}

// testToolID は検査で使うtool IDを返す。
func testToolID(t *testing.T) domain.ToolID {
	t.Helper()
	id, err := domain.ParseToolID("go")
	if err != nil {
		t.Fatalf("ParseToolID: %v", err)
	}
	return id
}

// testCatalogItem は導入可能なcatalog itemを返す。
func testCatalogItem(t *testing.T) store.CatalogItem {
	t.Helper()
	return store.CatalogItem{
		VersionText:         "1.25.0",
		Channel:             domain.ChannelStable,
		Lifecycle:           domain.LifecycleSupported,
		LifecycleAssessedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Installable:         true,
		ProviderKind:        store.ProviderOfficial,
		ProviderRelease:     "go1.25.0",
		ArtifactFile:        "go1.25.0.linux-amd64.tar.gz",
		ArtifactURL:         "https://go.dev/dl/go1.25.0.linux-amd64.tar.gz",
		ArtifactSize:        1024,
		ArtifactDigest:      testDigest(t),
		ChecksumSource:      store.ChecksumAssetField,
	}
}

// testDefinitionPlatform はprobeとstorageを1件ずつ持つplatform blockを返す。
func testDefinitionPlatform(t *testing.T) definition.Platform {
	t.Helper()
	return definition.Platform{
		Platform:     platformOf(t, "linux-amd64-glibc"),
		ArtifactKind: definition.KindOfficial,
		Provider: definition.Provider{
			Name:     "Go project",
			Homepage: "https://go.dev",
			License:  "BSD-3-Clause",
		},
		Artifact: definition.Artifact{Format: definition.FormatTarGz},
		Install:  definition.Install{StripComponents: 1},
		Storage: []definition.Storage{{
			ID:    "npm-cache",
			Kind:  definition.StorageBuildCache,
			Scope: definition.ScopeTool,
			Path:  "cache",
			Purge: definition.StorageExplicit,
		}},
		Runtime: definition.Runtime{Commands: []definition.Command{{
			Name:   "go",
			Target: "{{payload}}/bin/go",
			Args:   nil,
		}}},
		Validation: definition.Validation{Probes: []definition.Probe{{
			ID:              "go-version",
			RuntimeCommand:  "go",
			Args:            []string{"version"},
			Stream:          definition.StreamStdout,
			Expect:          definition.ExpectVersion,
			Regex:           `go(\d+\.\d+\.\d+)`,
			ExpectedVersion: "{{version}}",
			Timeout:         30 * time.Second,
			Required:        true,
		}}},
	}
}

// testPlanRequest は成功するPlan組立て入力を返す。
func testPlanRequest(t *testing.T) PlanRequest {
	t.Helper()
	invocation, err := domain.ParseInvocationID(strings.Repeat("a", 32))
	if err != nil {
		t.Fatalf("ParseInvocationID: %v", err)
	}
	operation, err := domain.ParseOperationID(strings.Repeat("b", 32))
	if err != nil {
		t.Fatalf("ParseOperationID: %v", err)
	}
	return PlanRequest{
		ClientVersion:       "2026.08.31.01",
		Invocation:          invocation,
		Operation:           operation,
		CreatedAt:           time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
		Tool:                testToolID(t),
		Item:                testCatalogItem(t),
		Platform:            testDefinitionPlatform(t),
		Roots:               testRoots(t, "linux-amd64-glibc"),
		ProbeTempRoot:       renderPathValue(t, domain.RoleStaging, probeTempParent),
		DownloadDestination: renderPathValue(t, domain.RoleDownloadCache, "/data/gdtvm/cache/downloads"),
		StagingDestination:  renderPathValue(t, domain.RoleStaging, "/data/gdtvm/tmp/staging"),
		Inputs: store.PlanInputs{
			RootID:               strings.Repeat("9", 32),
			ConfigSHA256:         strings.Repeat("c", 64),
			DefinitionSHA256:     strings.Repeat("d", 64),
			CatalogSHA256:        strings.Repeat("e", 64),
			RegistrySHA256:       strings.Repeat("f", 64),
			SelectionsRevision:   1,
			SetupRevision:        2,
			ReceiptIndexRevision: 3,
		},
	}
}

// TestBuildInstallPlanProducesEncodablePlan はPlanがcodecを通ることを固定する。
//
// **[store.EncodePlan]を通す。** P2-04が§16の全契約（key、enum、同値制約、
// ID順、承認要否）をcodecへ実装しているため、そこを通せば組立て側の誤りが
// codecの検査に当たる。builder専用の期待値を書き直すと、契約が2箇所に散る。
func TestBuildInstallPlanProducesEncodablePlan(t *testing.T) {
	plan, err := BuildInstallPlan(testPlanRequest(t))
	if err != nil {
		t.Fatalf("BuildInstallPlan = %v", err)
	}
	data, encodeErr := store.EncodePlan(plan)
	if encodeErr != nil {
		t.Fatalf("EncodePlan = %v", encodeErr)
	}
	// 読み直して同じ値になることまで見る。encodeだけだと、decodeできない形を
	// 作っても気付けない。
	parsed, parseErr := store.ParsePlan(data)
	if parseErr != nil {
		t.Fatalf("ParsePlan = %v", parseErr)
	}
	if parsed.Kind != store.OperationInstall {
		t.Errorf("operation = %s, want install", parsed.Kind)
	}
	if parsed.Setup != nil {
		// §8.1「setup/setup-removeのPlanだけは`SetupPlan`を必須とし、他operation
		// ではnullにする」。
		t.Error("install PlanにSetupPlanがある")
	}
	if len(parsed.Downloads) != 1 || len(parsed.Extracts) != 1 {
		t.Fatalf("downloads=%d extracts=%d, want 1/1",
			len(parsed.Downloads), len(parsed.Extracts))
	}
	if parsed.Extracts[0].SourceDownloadID != parsed.Downloads[0].ID {
		t.Errorf("extractのsource_download_id = %q, want %q",
			parsed.Extracts[0].SourceDownloadID, parsed.Downloads[0].ID)
	}
}

// TestBuildInstallPlanExpandsProbe はprobeが完全展開されることを固定する。
//
// docs/04-storage-and-data.md §16「definition probeを**完全展開**した値」。
// Planにtemplateが残るとExecuteが評価することになり、利用者が承認した文字列と
// 実際のargvが食い違いうる。
func TestBuildInstallPlanExpandsProbe(t *testing.T) {
	plan, err := BuildInstallPlan(testPlanRequest(t))
	if err != nil {
		t.Fatalf("BuildInstallPlan = %v", err)
	}
	if len(plan.Probes) != 1 {
		t.Fatalf("probes = %d件, want 1", len(plan.Probes))
	}
	probe := plan.Probes[0]
	if want := payloadRoot + "/bin/go"; probe.Executable.Path() != want {
		t.Errorf("executable = %q, want %q", probe.Executable.Path(), want)
	}
	if probe.ExpectedVersion != "1.25.0" {
		t.Errorf("expected_version = %q, want 1.25.0", probe.ExpectedVersion)
	}
	// docs/06-tool-definition.md §11「**probeのcwdはその probe temp とし、
	// 呼出し元のcurrent directoryを継承しない**」。docs/08-install-runtime.md
	// §7手順2も同じ。**payloadをcwdにしない**——payload内の`global.json`や
	// `.nvmrc`のようなfileがprobe結果を変えうる。
	wantCwd := probeTempParent + "/go-version"
	if probe.WorkingDirectory.Path() != wantCwd {
		t.Errorf("cwd = %q, want %q", probe.WorkingDirectory.Path(), wantCwd)
	}
	if probe.WorkingDirectory.Path() == payloadRoot {
		t.Error("cwdがpayload rootになっている（§11違反）")
	}
	// probeが書けるのはprobe tempだけである。
	if len(probe.WritePaths) != 1 || probe.WritePaths[0].Path() != wantCwd {
		t.Errorf("write_paths = %+v, want [%s]", probe.WritePaths, wantCwd)
	}
	if probe.TimeoutMillis != 30_000 {
		t.Errorf("timeout = %dms, want 30000", probe.TimeoutMillis)
	}
	// argsはentry 1件ごとにargv 1要素。literalはvalue、pathはPathValue。
	if len(probe.Args) != 1 {
		t.Fatalf("args = %d件, want 1", len(probe.Args))
	}
	if probe.Args[0].Kind != store.ArgLiteral || probe.Args[0].Value != "version" {
		t.Errorf("args[0] = %+v, want literal \"version\"", probe.Args[0])
	}
	// §16「完全version、artifact URL/digest、provider license、理由を空にしない」。
	if probe.Version == "" || probe.Source == "" || probe.License == "" {
		t.Errorf("probeのversion/source/licenseに空がある: %+v", probe)
	}
	if probe.ArtifactDigest.IsZero() || probe.ReasonMessageID.IsZero() {
		t.Error("probeのdigest/理由が未設定")
	}
	// Plan全体にtemplateが残っていないこと。
	data, encodeErr := store.EncodePlan(plan)
	if encodeErr != nil {
		t.Fatalf("EncodePlan = %v", encodeErr)
	}
	if strings.Contains(string(data), "{{") {
		t.Errorf("Planにtemplateが残っている: %s", data)
	}
}

// TestBuildInstallPlanSeparatesPathArgs はpath argがliteralへ埋まらないことを固定する。
//
// §16「definitionの1個のargs entryを複数argvへ分割せず、pathをliteralや
// warning parameterへ埋め込まない」。
func TestBuildInstallPlanSeparatesPathArgs(t *testing.T) {
	req := testPlanRequest(t)
	req.Platform.Runtime.Commands[0].Args = []string{"-C", "{{payload}}"}
	plan, err := BuildInstallPlan(req)
	if err != nil {
		t.Fatalf("BuildInstallPlan = %v", err)
	}
	args := plan.Probes[0].Args
	if len(args) != 3 {
		t.Fatalf("args = %d件, want 3（command 2件＋probe 1件）", len(args))
	}
	// command宣言argsが先、probe固有argsが後である。
	if args[0].Kind != store.ArgLiteral || args[0].Value != "-C" {
		t.Errorf("args[0] = %+v, want literal \"-C\"", args[0])
	}
	if args[1].Kind != store.ArgPath || args[1].Path.Path() != payloadRoot {
		t.Errorf("args[1] = %+v, want path %q", args[1], payloadRoot)
	}
	if args[1].Value != "" {
		t.Errorf("kind=pathにvalueが入っている: %q", args[1].Value)
	}
	if args[2].Kind != store.ArgLiteral || args[2].Value != "version" {
		t.Errorf("args[2] = %+v, want literal \"version\"", args[2])
	}
}

// TestBuildInstallPlanRejectsUndeclaredCommand は宣言外commandを拒否することを固定する。
//
// §11のprobeは§10.1で宣言したcommandだけを起動できる。宣言に無い名前を許すと、
// Planが定義されていないprogramの起動を要求できる。
//
// **errorが未宣言のcommand名を挙げることまで見る。** 「errorになった」だけだと、
// 未宣言時にzero値の[definition.Command]が返り、その空targetを[RenderPath]が
// 拒否する経路でも通ってしまう。それでは名前の照合そのものを固定できない。
func TestBuildInstallPlanRejectsUndeclaredCommand(t *testing.T) {
	req := testPlanRequest(t)
	req.Platform.Validation.Probes[0].RuntimeCommand = "sh"
	_, err := BuildInstallPlan(req)
	if err == nil {
		t.Fatal("宣言外のruntime commandが通った")
	}
	if !strings.Contains(err.Error(), `"sh"`) {
		t.Errorf("errorが未宣言のcommand名を挙げていない: %v", err)
	}
}

// TestBuildInstallPlanRequiresProbeTempPerProbe はprobe tempの割当てを固定する。
//
// docs/06-tool-definition.md §11「**probeごとに**空のowner-only probe tempを作り、
// 成功/失敗/cancel後にengineが削除する」。probe間で共有すると、先に走ったprobeが
// 残したfileが後のprobeの結果を変えうる。
func TestBuildInstallPlanRequiresProbeTempPerProbe(t *testing.T) {
	t.Run("probeがあるのにrootが無ければ拒否する", func(t *testing.T) {
		req := testPlanRequest(t)
		req.ProbeTempRoot = domain.PathValue{}
		if _, err := BuildInstallPlan(req); err == nil {
			t.Fatal("probe temp root無しでPlanが作れた")
		}
	})
	t.Run("probeが無ければrootは要らない", func(t *testing.T) {
		req := testPlanRequest(t)
		req.ProbeTempRoot = domain.PathValue{}
		req.Platform.Validation.Probes = nil
		if _, err := BuildInstallPlan(req); err != nil {
			t.Fatalf("probe無しで拒否された: %v", err)
		}
	})
	t.Run("probeごとに別のdirectoryを割り当てる", func(t *testing.T) {
		req := testPlanRequest(t)
		second := req.Platform.Validation.Probes[0]
		second.ID = "go-env"
		req.Platform.Validation.Probes = append(req.Platform.Validation.Probes, second)

		plan, err := BuildInstallPlan(req)
		if err != nil {
			t.Fatalf("BuildInstallPlan = %v", err)
		}
		if len(plan.Probes) != 2 {
			t.Fatalf("probes = %d件, want 2", len(plan.Probes))
		}
		first, other := plan.Probes[0].WorkingDirectory.Path(), plan.Probes[1].WorkingDirectory.Path()
		if first == other {
			t.Errorf("probe間でtempを共有している: %q", first)
		}
		if first != probeTempParent+"/go-version" || other != probeTempParent+"/go-env" {
			t.Errorf("probe temp = %q, %q", first, other)
		}
	})
	t.Run("rootのroleが違えば拒否する", func(t *testing.T) {
		req := testPlanRequest(t)
		req.ProbeTempRoot = renderPathValue(t, domain.RolePayload, probeTempParent)
		if _, err := BuildInstallPlan(req); err == nil {
			t.Fatal("staging以外のroleが通った")
		}
	})
}

// TestBuildInstallPlanResolvesProbeTempTemplate は`{{probe_temp}}`がprobe固有の
// directoryへ解決することを固定する。
func TestBuildInstallPlanResolvesProbeTempTemplate(t *testing.T) {
	req := testPlanRequest(t)
	req.Platform.Validation.Probes[0].Args = []string{"{{probe_temp}}/out"}
	plan, err := BuildInstallPlan(req)
	if err != nil {
		t.Fatalf("BuildInstallPlan = %v", err)
	}
	args := plan.Probes[0].Args
	if len(args) != 1 || args[0].Kind != store.ArgPath {
		t.Fatalf("args = %+v, want path 1件", args)
	}
	if want := probeTempParent + "/go-version/out"; args[0].Path.Path() != want {
		t.Errorf("probe temp arg = %q, want %q", args[0].Path.Path(), want)
	}
}

// TestBuildInstallPlanOmitsInternalWrites はdata root内部を`writes[]`へ出さないことを固定する。
//
// §16「staging、download cache、state、receipt、index、shim、storageなど
// data root内部の書込みはPlanへ列挙せず、Executeの封じ込め検査で保証する」。
func TestBuildInstallPlanOmitsInternalWrites(t *testing.T) {
	plan, err := BuildInstallPlan(testPlanRequest(t))
	if err != nil {
		t.Fatalf("BuildInstallPlan = %v", err)
	}
	if len(plan.Writes) != 0 {
		t.Errorf("writes = %+v, want 空", plan.Writes)
	}
	// storageは`storage[]`へ出る。`writes[]`へ二重に出さない。
	if len(plan.Storage) != 1 {
		t.Fatalf("storage = %d件, want 1", len(plan.Storage))
	}
	if plan.Storage[0].Action != store.StorageCreate {
		t.Errorf("storage action = %s, want create", plan.Storage[0].Action)
	}
	if plan.Storage[0].Target.Path() != cacheRoot {
		t.Errorf("storage target = %q, want %q", plan.Storage[0].Target.Path(), cacheRoot)
	}
}

// TestBuildInstallPlanBuildsWarnings は§16.1のwarningを固定する。
func TestBuildInstallPlanBuildsWarnings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PlanRequest)
		want   []store.PlanWarningCode
	}{
		{"official・stable・supported", func(*PlanRequest) {}, nil},
		{"third-party", func(r *PlanRequest) {
			r.Platform.ArtifactKind = definition.KindThirdParty
			r.Platform.Provider.AdoptionReason = "provider.python.standalone_reason"
		}, []store.PlanWarningCode{store.WarnThirdParty}},
		{"prerelease", func(r *PlanRequest) {
			r.Item.Channel = domain.ChannelPrerelease
		}, []store.PlanWarningCode{store.WarnPrerelease}},
		{"eol", func(r *PlanRequest) {
			r.Item.Lifecycle = domain.LifecycleEOL
			r.Item.LifecycleEvidence = "https://go.dev/doc/devel/release"
		}, []store.PlanWarningCode{store.WarnEOL}},
		{"lifecycle=unknownでは立てない", func(r *PlanRequest) {
			r.Item.Lifecycle = domain.LifecycleUnknown
		}, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := testPlanRequest(t)
			test.mutate(&req)
			plan, err := BuildInstallPlan(req)
			if err != nil {
				t.Fatalf("BuildInstallPlan = %v", err)
			}
			var got []store.PlanWarningCode
			for _, warning := range plan.Warnings {
				got = append(got, warning.Code)
			}
			if len(got) != len(test.want) {
				t.Fatalf("warnings = %v, want %v", got, test.want)
			}
			for index := range test.want {
				if got[index] != test.want[index] {
					t.Errorf("warnings = %v, want %v", got, test.want)
				}
			}
			// §16「`warning_count`と`warnings`の件数を一致させる」。
			if plan.Summary.WarningCount != int64(len(plan.Warnings)) {
				t.Errorf("warning_count = %d, warnings = %d件",
					plan.Summary.WarningCount, len(plan.Warnings))
			}
		})
	}
}

// TestBuildInstallPlanWarnsOnLicenseNotice はlicense noticeのwarningを固定する。
//
// docs/10-security.md §8「公式配布物でもOSI承認OSS licenseでないplatformには
// `license_notice`を宣言し、Planの重要要約で明示承認を求める」。
func TestBuildInstallPlanWarnsOnLicenseNotice(t *testing.T) {
	req := testPlanRequest(t)
	notice, err := domain.ParseMessageID("license.dotnet.windows_library_license")
	if err != nil {
		t.Fatalf("ParseMessageID: %v", err)
	}
	req.Platform.LicenseNotice = notice

	plan, buildErr := BuildInstallPlan(req)
	if buildErr != nil {
		t.Fatalf("BuildInstallPlan = %v", buildErr)
	}
	if len(plan.Warnings) != 1 || plan.Warnings[0].Code != store.WarnRestrictiveLicense {
		t.Fatalf("warnings = %+v, want W_RESTRICTIVE_LICENSE", plan.Warnings)
	}
	if !plan.Warnings[0].RequiresExplicitApproval {
		t.Error("W_RESTRICTIVE_LICENSEが明示承認を要求していない")
	}
	if plan.Summary.LicenseNotice != notice.String() {
		t.Errorf("summary.license_notice = %q, want %q",
			plan.Summary.LicenseNotice, notice.String())
	}
}

// TestBuildInstallPlanRejectsInvalidRequest は前提違反を拒否することを固定する。
func TestBuildInstallPlanRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PlanRequest)
	}{
		{"client versionが空", func(r *PlanRequest) { r.ClientVersion = "" }},
		{"invocation IDが未設定", func(r *PlanRequest) { r.Invocation = domain.InvocationID{} }},
		{"operation IDが未設定", func(r *PlanRequest) { r.Operation = domain.OperationID{} }},
		{"作成時刻が未設定", func(r *PlanRequest) { r.CreatedAt = time.Time{} }},
		{"作成時刻がUTCでない", func(r *PlanRequest) {
			r.CreatedAt = r.CreatedAt.In(time.FixedZone("JST", 9*3600))
		}},
		{"tool IDが未設定", func(r *PlanRequest) { r.Tool = domain.ToolID{} }},
		{"versionが空", func(r *PlanRequest) { r.Item.VersionText = "" }},
		// §3.1が「導入できないversionをPlanにしない」と定める。
		{"installable=false", func(r *PlanRequest) { r.Item.Installable = false }},
		{"artifact URLが空", func(r *PlanRequest) { r.Item.ArtifactURL = "" }},
		{"artifact file名が空", func(r *PlanRequest) { r.Item.ArtifactFile = "" }},
		// docs/10-security.md §8がupstream digestを必須にする。
		{"digestが無い", func(r *PlanRequest) { r.Item.ArtifactDigest = domain.Digest{} }},
		{"downloadの保存先が未設定", func(r *PlanRequest) {
			r.DownloadDestination = domain.PathValue{}
		}},
		{"downloadの保存先roleが違う", func(r *PlanRequest) {
			r.DownloadDestination = renderPathValue(t, domain.RolePayload, "/data/x")
		}},
		{"展開先が未設定", func(r *PlanRequest) { r.StagingDestination = domain.PathValue{} }},
		{"展開先roleが違う", func(r *PlanRequest) {
			r.StagingDestination = renderPathValue(t, domain.RoleDownloadCache, "/data/x")
		}},
		{"payload rootが未設定", func(r *PlanRequest) { r.Roots.Payload = domain.PathValue{} }},
		{"host platformが未設定", func(r *PlanRequest) { r.Roots.Host = domain.Platform{} }},
		{"storage rootが渡されていない", func(r *PlanRequest) { r.Roots.Storage = nil }},
		{"artifact kindが未設定", func(r *PlanRequest) { r.Platform.ArtifactKind = "" }},
		{"archive formatが未知", func(r *PlanRequest) {
			r.Platform.Artifact.Format = definition.ArchiveFormat("7z")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := testPlanRequest(t)
			test.mutate(&req)
			if _, err := BuildInstallPlan(req); err == nil {
				t.Fatal("前提違反が通った")
			}
		})
	}
}

// TestBuildInstallPlanIsDeterministic は同じ入力から同じPlanが出ることを固定する。
//
// 純関数であることを担保する。時刻とIDを呼出し側から受けているため、
// builder内部にnondeterminismが入る余地は無いはずである。
func TestBuildInstallPlanIsDeterministic(t *testing.T) {
	req := testPlanRequest(t)
	first, err := BuildInstallPlan(req)
	if err != nil {
		t.Fatalf("1回目 = %v", err)
	}
	second, err := BuildInstallPlan(req)
	if err != nil {
		t.Fatalf("2回目 = %v", err)
	}
	firstData, encodeErr := store.EncodePlan(first)
	if encodeErr != nil {
		t.Fatalf("EncodePlan = %v", encodeErr)
	}
	secondData, encodeErr := store.EncodePlan(second)
	if encodeErr != nil {
		t.Fatalf("EncodePlan = %v", encodeErr)
	}
	if string(firstData) != string(secondData) {
		t.Error("同じ入力から違うPlanが出た")
	}
}

// TestBuildInstallPlanCarriesInputs は`inputs`をそのまま載せることを固定する。
//
// §16のrevision/digestは供給元が計算した値である。builderが作り直すと、
// 作成時とExecuteの照合時で読む経路が同じになり`E_PLAN_STALE`が退化する。
func TestBuildInstallPlanCarriesInputs(t *testing.T) {
	req := testPlanRequest(t)
	plan, err := BuildInstallPlan(req)
	if err != nil {
		t.Fatalf("BuildInstallPlan = %v", err)
	}
	if plan.Inputs != req.Inputs {
		t.Errorf("inputs = %+v, want %+v", plan.Inputs, req.Inputs)
	}
}
