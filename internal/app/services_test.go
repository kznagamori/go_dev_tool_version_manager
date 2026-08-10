package app

import (
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

// validBuild はdocs/11-quality-and-ci.md §4の全項目を満たすrelease build metadataを返す。
func validBuild() BuildInfo {
	return BuildInfo{
		ClientVersion:    "2026.08.10.00",
		ClientRelease:    true,
		Commit:           strings.Repeat("a1b2", 10),
		Dirty:            false,
		BuiltAt:          time.Date(2026, time.August, 10, 3, 35, 34, 0, time.UTC),
		GoToolchain:      "go1.26.5",
		GOOS:             "linux",
		GOARCH:           "amd64",
		CGOEnabled:       false,
		DefinitionSchema: 1,
		RegistrySchema:   1,
		StateSchema:      1,
		RepositoryOwner:  "kznagamori",
		RepositoryName:   "go_dev_tool_version_manager",
	}
}

func TestNewServicesAcceptsValidInput(t *testing.T) {
	set := fake.NewSet()

	services, err := NewServices(validBuild(), set.Ports())
	if err != nil {
		t.Fatalf("NewServices = %v, want nil", err)
	}
	if got := services.BuildInfo(); got != validBuild() {
		t.Errorf("BuildInfo = %+v, want %+v", got, validBuild())
	}
	if len(services.ports.Missing()) != 0 {
		t.Errorf("組み立てたServicesのportが欠けている: %v", services.ports.Missing())
	}
}

// TestNewServicesTouchesNoPort はconstructorが外部作用を起こさないことを検査する。
//
// docs/02-architecture.md §4は「constructorは依存の存在とbuild metadata形式だけを
// 検査し、filesystem/network変更を行わない」と定める。全外部作用はportを通るため
// （§1）、fakeのInjectorに1件も記録が無いことがその証明になる。
func TestNewServicesTouchesNoPort(t *testing.T) {
	set := fake.NewSet()

	if _, err := NewServices(validBuild(), set.Ports()); err != nil {
		t.Fatalf("NewServices = %v, want nil", err)
	}
	if ops := set.Injector.Operations(); len(ops) != 0 {
		t.Errorf("constructorがportを呼んだ: %v", ops)
	}
}

func TestNewServicesRejectsMissingPort(t *testing.T) {
	tests := []struct {
		name    string
		clear   func(*port.Ports)
		wantAll []string
	}{
		{"clock", func(p *port.Ports) { p.Clock = nil }, []string{"Clock"}},
		{"filesystem", func(p *port.Ports) { p.FileSystem = nil }, []string{"FileSystem"}},
		{"http", func(p *port.Ports) { p.HTTPClient = nil }, []string{"HTTPClient"}},
		{"link", func(p *port.Ports) { p.LinkManager = nil }, []string{"LinkManager"}},
		{"logger", func(p *port.Ports) { p.Logger = nil }, []string{"Logger"}},
		{"process", func(p *port.Ports) { p.ProcessRunner = nil }, []string{"ProcessRunner"}},
		{"random", func(p *port.Ports) { p.Random = nil }, []string{"Random"}},
		{"userlookup", func(p *port.Ports) { p.UserLookup = nil }, []string{"UserLookup"}},
		{
			"すべて未設定",
			func(p *port.Ports) { *p = port.Ports{} },
			[]string{
				"Clock", "FileSystem", "HTTPClient", "LinkManager",
				"Logger", "ProcessRunner", "Random", "UserLookup",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ports := fake.NewSet().Ports()
			test.clear(&ports)

			_, err := NewServices(validBuild(), ports)
			if err == nil {
				t.Fatal("NewServices = nil, want error")
			}
			for _, want := range test.wantAll {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q に %q が含まれない", err, want)
				}
			}
		})
	}
}

// TestNewServicesReportsMissingPortsInDeclarationOrder は診断の順序を固定する。
//
// 欠落portの列挙順がmap iterationのように実行ごとに変わると、CI logやbug報告の
// 突き合わせができなくなる。
func TestNewServicesReportsMissingPortsInDeclarationOrder(t *testing.T) {
	_, err := NewServices(validBuild(), port.Ports{})
	if err == nil {
		t.Fatal("NewServices = nil, want error")
	}
	const want = "Clock, FileSystem, HTTPClient, LinkManager, Logger, ProcessRunner, Random, UserLookup"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q に %q が含まれない", err, want)
	}
}

func TestNewServicesRejectsInvalidBuildInfo(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*BuildInfo)
		wantSub string
	}{
		{"version空", func(b *BuildInfo) { b.ClientVersion = "" }, "client version"},
		{"CalVer以外", func(b *BuildInfo) { b.ClientVersion = "0.1.0" }, "client version"},
		{"leading v", func(b *BuildInfo) { b.ClientVersion = "v2026.08.10.00" }, "client version"},
		{"通番1桁", func(b *BuildInfo) { b.ClientVersion = "2026.08.10.0" }, "client version"},
		{"月が範囲外", func(b *BuildInfo) { b.ClientVersion = "2026.13.01.00" }, "client version"},
		{"実在しない日", func(b *BuildInfo) { b.ClientVersion = "2026.02.30.00" }, "実在しない"},
		{
			"releaseがdevelを名乗る",
			func(b *BuildInfo) { b.ClientVersion = devVersion },
			"development buildだけ",
		},
		{"release binaryがdirty", func(b *BuildInfo) { b.Dirty = true }, "dirty=false"},
		{"commitが短い", func(b *BuildInfo) { b.Commit = "a1b2c3" }, "commit"},
		{"commitが大文字hex", func(b *BuildInfo) { b.Commit = strings.Repeat("A1B2", 10) }, "commit"},
		{"build時刻が未設定", func(b *BuildInfo) { b.BuiltAt = time.Time{} }, "build時刻"},
		{
			"build時刻がUTCでない",
			func(b *BuildInfo) { b.BuiltAt = b.BuiltAt.In(time.FixedZone("JST", 9*60*60)) },
			"UTC",
		},
		{"toolchain形式違反", func(b *BuildInfo) { b.GoToolchain = "1.26.5" }, "toolchain"},
		{"toolchain空", func(b *BuildInfo) { b.GoToolchain = "" }, "toolchain"},
		{"GOOSが対象外", func(b *BuildInfo) { b.GOOS = "darwin" }, "build target"},
		{"GOARCHが対象外", func(b *BuildInfo) { b.GOARCH = "arm64" }, "build target"},
		{"CGO有効", func(b *BuildInfo) { b.CGOEnabled = true }, "CGO"},
		{"definition schemaが0", func(b *BuildInfo) { b.DefinitionSchema = 0 }, "definition schema"},
		{"registry schemaが負", func(b *BuildInfo) { b.RegistrySchema = -1 }, "registry schema"},
		{"state schemaが0", func(b *BuildInfo) { b.StateSchema = 0 }, "state schema"},
		{"owner空", func(b *BuildInfo) { b.RepositoryOwner = "" }, "repository owner"},
		{"nameに区切り", func(b *BuildInfo) { b.RepositoryName = "owner/name" }, "repository name"},
		{"nameに空白", func(b *BuildInfo) { b.RepositoryName = "go dev" }, "repository name"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			build := validBuild()
			test.mutate(&build)

			_, err := NewServices(build, fake.NewSet().Ports())
			if err == nil {
				t.Fatal("NewServices = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantSub) {
				t.Errorf("error %q に %q が含まれない", err, test.wantSub)
			}
		})
	}
}

// TestBuildInfoValidateAcceptsDevelopmentBuild はdocs/11-quality-and-ci.md §2が
// development buildだけへ許す組合せを確認する。
func TestBuildInfoValidateAcceptsDevelopmentBuild(t *testing.T) {
	build := validBuild()
	build.ClientVersion = devVersion
	build.ClientRelease = false
	build.Dirty = true

	if err := build.Validate(); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}

// TestBuildInfoValidateReportsEveryDefect は誤りを1件目で打ち切らないことを見る。
func TestBuildInfoValidateReportsEveryDefect(t *testing.T) {
	build := BuildInfo{}

	err := build.Validate()
	if err == nil {
		t.Fatal("Validate = nil, want error")
	}
	for _, want := range []string{
		"client version", "commit", "build時刻", "toolchain",
		"build target", "definition schema", "registry schema", "state schema",
		"repository owner", "repository name",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error に %q が含まれない:\n%v", want, err)
		}
	}
}

// TestServicesKeepsOwnBuildInfoCopy は生成後の呼出し側の書換えが伝わらないことを見る。
//
// docs/02-architecture.md §5が境界通過後の値をimmutableとして扱うと定めるため、
// 共有ではなくcopyであることを固定する。
func TestServicesKeepsOwnBuildInfoCopy(t *testing.T) {
	build := validBuild()

	services, err := NewServices(build, fake.NewSet().Ports())
	if err != nil {
		t.Fatalf("NewServices = %v, want nil", err)
	}

	build.ClientVersion = "2030.01.01.99"
	if got := services.BuildInfo().ClientVersion; got != "2026.08.10.00" {
		t.Errorf("ClientVersion = %q, want %q（呼出し側の書換えが伝わっている）", got, "2026.08.10.00")
	}

	fromService := services.BuildInfo()
	fromService.ClientVersion = "2031.01.01.99"
	if got := services.BuildInfo().ClientVersion; got != "2026.08.10.00" {
		t.Errorf("ClientVersion = %q, want %q（戻り値の書換えが伝わっている）", got, "2026.08.10.00")
	}
}

// TestServicesInstancesAreIndependent は2つのServicesが状態を共有しないことを見る。
//
// docs/02-architecture.md §12が「package global singletonを置かない」と定めるため、
// 別々に組み立てたinstanceが別のportを見ることを固定する。
func TestServicesInstancesAreIndependent(t *testing.T) {
	first, err := NewServices(validBuild(), fake.NewSet().Ports())
	if err != nil {
		t.Fatalf("1つ目のNewServices = %v, want nil", err)
	}
	second, err := NewServices(validBuild(), fake.NewSet().Ports())
	if err != nil {
		t.Fatalf("2つ目のNewServices = %v, want nil", err)
	}

	if first.ports.Clock == second.ports.Clock {
		t.Error("別々に組み立てたServicesが同じClockを共有している")
	}
	if first.ports.FileSystem == second.ports.FileSystem {
		t.Error("別々に組み立てたServicesが同じFileSystemを共有している")
	}
}
