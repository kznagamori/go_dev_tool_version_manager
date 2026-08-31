package app

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

const (
	scopeDataRoot     = "/data/gdtvm"
	scopeDistRoot     = "/data/gdtvm/dist"
	scopePayload      = "/data/gdtvm/tools/go/1.25.0/payload"
	scopeProfile      = "/home/u/.bashrc"
	scopeOutsidePath  = "/etc/passwd"
	scopeArtifactURL  = "https://go.dev/dl/go1.25.0.linux-amd64.tar.gz"
	scopeOtherHostURL = "https://evil.example/go.tar.gz"
)

// testPlanForScope はprobe 1件とdownload 1件を持つinstall Planを返す。
func testPlanForScope(t *testing.T) store.Plan {
	t.Helper()
	return store.Plan{
		Kind: store.OperationInstall,
		Downloads: []store.PlanDownload{{
			ID:  "artifact",
			URL: scopeArtifactURL,
		}},
		Probes: []store.PlanProbe{{
			ID:         "go-version",
			Executable: pathValueOf(t, domain.RolePayload, scopePayload+"/bin/go"),
			Args: []store.PlanArg{
				{Kind: store.ArgLiteral, Value: "version"},
			},
			WorkingDirectory: pathValueOf(t, domain.RolePayload, scopePayload),
		}},
	}
}

// testScopeRequest は成功するScope導出入力を返す。
func testScopeRequest(t *testing.T) PlanScopeRequest {
	t.Helper()
	return PlanScopeRequest{
		Plan:             testPlanForScope(t),
		DataRoot:         pathValueOf(t, domain.RoleDataRoot, scopeDataRoot),
		DistributionRoot: pathValueOf(t, domain.RoleDistributionRoot, scopeDistRoot),
		Host:             hostOf(t, "linux-amd64-glibc"),
	}
}

// TestScopeFromPlanAllowsContainmentRange は§8手順5の封じ込め範囲を固定する。
//
// docs/02-architecture.md §8手順5「全書込みがdata root、distribution root、
// 宣言済みintegration対象、project fileの中にあり」。
//
// **`writes[]`だけをrootにできない。** docs/04-storage-and-data.md §16が
// 「staging、download cache、state、receipt、index、shimなどdata root内部の
// 書込みはPlanへ列挙しない」と定めるため、`writes[]`だけだと列挙されない内部
// 書込みがすべて拒否される。
func TestScopeFromPlanAllowsContainmentRange(t *testing.T) {
	scope, err := ScopeFromPlan(testScopeRequest(t))
	if err != nil {
		t.Fatalf("ScopeFromPlan = %v", err)
	}
	tests := []struct {
		name string
		path string
		want bool
	}{
		// §16が`writes[]`へ出さないと定めた内部書込みが、いずれも通ること。
		{"payload", scopePayload + "/bin/go", true},
		{"staging", scopeDataRoot + "/tmp/staging/x", true},
		{"download cache", scopeDataRoot + "/cache/downloads/go.tar.gz", true},
		{"receipt", scopeDataRoot + "/receipts/go/1.25.0.json", true},
		{"shim", scopeDataRoot + "/shims/go", true},
		{"distribution root", scopeDistRoot + "/bin/gdtvm", true},
		// 範囲外は拒否する。
		{"data root外", scopeOutsidePath, false},
		{"data rootのprefixが一致するだけ", scopeDataRoot + "-other/x", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, allowed := scope.AllowsWrite(test.path); allowed != test.want {
				t.Errorf("AllowsWrite(%q) = %v, want %v", test.path, allowed, test.want)
			}
		})
	}
}

// TestScopeFromPlanAddsVisibleWriteTargets は`writes[]`のtargetがrootへ入ることを固定する。
//
// integration対象とproject fileはdata root外にあるため、`writes[]`が挙げた
// ものだけを追加で許す。
func TestScopeFromPlanAddsVisibleWriteTargets(t *testing.T) {
	req := testScopeRequest(t)
	req.Plan.Writes = []store.PlanWrite{{
		ID:     "shell-profile",
		Action: store.WriteReplace,
		Target: pathValueOf(t, domain.RoleConfig, scopeProfile),
	}}
	scope, err := ScopeFromPlan(req)
	if err != nil {
		t.Fatalf("ScopeFromPlan = %v", err)
	}
	if _, allowed := scope.AllowsWrite(scopeProfile); !allowed {
		t.Errorf("宣言したintegration対象 %q が拒否された", scopeProfile)
	}
	// 宣言していない兄弟fileは通らない。
	if _, allowed := scope.AllowsWrite("/home/u/.zshrc"); allowed {
		t.Error("宣言していないprofileが通った")
	}
}

// TestScopeFromPlanSkipsRegistryValue はregistry valueをfilesystem rootにしないことを固定する。
//
// docs/04-storage-and-data.md §17.2「Windows user PATHのregistry valueは
// filesystem pathではないが変更対象の識別が必要なため…`path`はexact locator
// `HKCU\Environment\Path`とする。これはPlan `PathValue.path`をabsolute
// filesystem pathとしない唯一の例外である」。
//
// filesystem書込みのrootとして扱うと、その文字列で始まるpathを許すことになる。
func TestScopeFromPlanSkipsRegistryValue(t *testing.T) {
	req := testScopeRequest(t)
	req.Host = hostOf(t, "windows-amd64")
	req.DataRoot = pathValueOf(t, domain.RoleDataRoot, `C:\data\gdtvm`)
	req.DistributionRoot = pathValueOf(t, domain.RoleDistributionRoot, `C:\data\gdtvm\dist`)
	req.Plan.Probes = nil
	locator := `HKCU\Environment\Path`
	req.Plan.Writes = []store.PlanWrite{{
		ID:     "user-path",
		Action: store.WriteRegistryValue,
		Target: pathValueOf(t, domain.RoleConfig, locator),
	}}

	scope, err := ScopeFromPlan(req)
	if err != nil {
		t.Fatalf("ScopeFromPlan = %v", err)
	}
	if _, allowed := scope.AllowsWrite(locator); allowed {
		t.Error("registry locatorがfilesystem rootとして許された")
	}
	if _, allowed := scope.AllowsWrite(locator + `\sub`); allowed {
		t.Error("registry locator配下がfilesystem rootとして許された")
	}
	// data rootは通常どおり許す。
	if _, allowed := scope.AllowsWrite(`C:\data\gdtvm\receipts\go.json`); !allowed {
		t.Error("data rootが拒否された")
	}
}

// TestScopeFromPlanAllowsOnlyListedProcesses は任意helper processの拒否を固定する。
//
// docs/02-architecture.md §8手順5「任意helper/backend processを起動しない」。
// 許可listがPlanのprobeと完全一致することでこれを表す。
func TestScopeFromPlanAllowsOnlyListedProcesses(t *testing.T) {
	scope, err := ScopeFromPlan(testScopeRequest(t))
	if err != nil {
		t.Fatalf("ScopeFromPlan = %v", err)
	}
	executable := scopePayload + "/bin/go"
	if !scope.AllowsProcess(executable, []string{"version"}, scopePayload) {
		t.Error("Planのprobeが拒否された")
	}
	tests := []struct {
		name       string
		executable string
		args       []string
		dir        string
	}{
		{"別のexecutable", "/bin/sh", []string{"version"}, scopePayload},
		// argsは完全一致である。prefix一致だと、宣言したargsの後ろへ引数を
		// 足して別の動作をさせられる。
		{"argsが多い", executable, []string{"version", "-json"}, scopePayload},
		{"argsが少ない", executable, nil, scopePayload},
		{"argsが違う", executable, []string{"env"}, scopePayload},
		{"cwdが違う", executable, []string{"version"}, "/tmp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if scope.AllowsProcess(test.executable, test.args, test.dir) {
				t.Error("Plan外のprocessが許された")
			}
		})
	}
}

// TestScopeFromPlanWithoutProbesAllowsNoProcess はprobe 0件で1つも起動できないことを固定する。
func TestScopeFromPlanWithoutProbesAllowsNoProcess(t *testing.T) {
	req := testScopeRequest(t)
	req.Plan.Probes = nil
	scope, err := ScopeFromPlan(req)
	if err != nil {
		t.Fatalf("ScopeFromPlan = %v", err)
	}
	if scope.AllowsProcess("/bin/sh", nil, "/") {
		t.Error("probeが無いのにprocessが許された")
	}
}

// TestScopeFromPlanAllowsOnlyListedDownloads はPlan外downloadの拒否を固定する。
func TestScopeFromPlanAllowsOnlyListedDownloads(t *testing.T) {
	scope, err := ScopeFromPlan(testScopeRequest(t))
	if err != nil {
		t.Fatalf("ScopeFromPlan = %v", err)
	}
	if !scope.AllowsDownload(scopeArtifactURL) {
		t.Error("Planのdownloadが拒否された")
	}
	if scope.AllowsDownload(scopeOtherHostURL) {
		t.Error("Plan外のdownloadが許された")
	}
}

// TestScopeFromPlanConvertsPathArgs はkind=pathのargvをnative pathへ戻すことを固定する。
//
// §16「`kind=path`では`value`を空、`path`を非空の`PathValue`とし、そのnative
// pathをargv 1要素とする」。ここで戻し方を誤ると、Planが承認したargvと
// 照合対象が食い違う。
func TestScopeFromPlanConvertsPathArgs(t *testing.T) {
	req := testScopeRequest(t)
	req.Plan.Probes[0].Args = []store.PlanArg{
		{Kind: store.ArgLiteral, Value: "-C"},
		{Kind: store.ArgPath, Path: pathValueOf(t, domain.RolePayload, scopePayload)},
	}
	scope, err := ScopeFromPlan(req)
	if err != nil {
		t.Fatalf("ScopeFromPlan = %v", err)
	}
	executable := scopePayload + "/bin/go"
	if !scope.AllowsProcess(executable, []string{"-C", scopePayload}, scopePayload) {
		t.Error("path argを含むprobeが拒否された")
	}
}

// TestScopeFromPlanRejectsInvalidInput は前提違反を拒否することを固定する。
func TestScopeFromPlanRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PlanScopeRequest)
	}{
		{"hostが未設定", func(r *PlanScopeRequest) { r.Host = domain.Platform{} }},
		{"data rootが未設定", func(r *PlanScopeRequest) { r.DataRoot = domain.PathValue{} }},
		{"data rootのroleが違う", func(r *PlanScopeRequest) {
			r.DataRoot = pathValueOf(t, domain.RolePayload, scopeDataRoot)
		}},
		{"distribution rootが未設定", func(r *PlanScopeRequest) {
			r.DistributionRoot = domain.PathValue{}
		}},
		{"distribution rootのroleが違う", func(r *PlanScopeRequest) {
			r.DistributionRoot = pathValueOf(t, domain.RoleConfig, scopeDistRoot)
		}},
		// 未設定targetは[NewScope]のzero root検査でも落ちる。ここで先に見るのは
		// どのwrites entryかを診断へ出すためで、その診断は
		// TestScopeFromPlanNamesInvalidWriteが別途固定する。
		{"writesのtargetが未設定", func(r *PlanScopeRequest) {
			r.Plan.Writes = []store.PlanWrite{{ID: "x", Action: store.WriteCreate}}
		}},
		{"probeのexecutableが未設定", func(r *PlanScopeRequest) {
			r.Plan.Probes[0].Executable = domain.PathValue{}
		}},
		{"kind=literalにpathがある", func(r *PlanScopeRequest) {
			r.Plan.Probes[0].Args = []store.PlanArg{{
				Kind: store.ArgLiteral, Value: "x",
				Path: pathValueOf(t, domain.RolePayload, scopePayload),
			}}
		}},
		{"kind=pathにvalueがある", func(r *PlanScopeRequest) {
			r.Plan.Probes[0].Args = []store.PlanArg{{
				Kind: store.ArgPath, Value: "x",
				Path: pathValueOf(t, domain.RolePayload, scopePayload),
			}}
		}},
		{"kind=pathのpathが空", func(r *PlanScopeRequest) {
			r.Plan.Probes[0].Args = []store.PlanArg{{Kind: store.ArgPath}}
		}},
		{"未知のarg kind", func(r *PlanScopeRequest) {
			r.Plan.Probes[0].Args = []store.PlanArg{{Kind: store.PlanArgKind("shell")}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := testScopeRequest(t)
			test.mutate(&req)
			if _, err := ScopeFromPlan(req); err == nil {
				t.Fatal("前提違反が通った")
			}
		})
	}
}

// TestScopeFromPlanNamesInvalidWrite は不正なwrites entryを診断が特定することを固定する。
//
// zero targetは[NewScope]のzero root検査でも落ちるため、封じ込めの保証自体は
// そちらが担う。ここで先に見る意味は「どのentryが悪いか」を出すことにあり、
// それが失われていないことを見る。
func TestScopeFromPlanNamesInvalidWrite(t *testing.T) {
	req := testScopeRequest(t)
	req.Plan.Writes = []store.PlanWrite{
		{ID: "ok", Action: store.WriteCreate,
			Target: pathValueOf(t, domain.RoleConfig, scopeProfile)},
		{ID: "broken", Action: store.WriteCreate},
	}
	_, err := ScopeFromPlan(req)
	if err == nil {
		t.Fatal("target未設定のwrites entryが通った")
	}
	if !strings.Contains(err.Error(), "writes[1]") {
		t.Errorf("診断が問題のentryを特定していない: %v", err)
	}
}
