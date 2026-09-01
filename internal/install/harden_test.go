package install

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

const hardenPayloadDir = "/data/gdtvm/tmp/operations/op1/payload"

// newHardenHarness はpayload treeを持つfake filesystemを用意する。
func newHardenHarness(t *testing.T) (*fake.FileSystem, *fake.Injector) {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	if err := filesystem.MkdirAll(hardenPayloadDir+"/bin", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// owner executeを持つfile（§6が展開時に保持する）。
	filesystem.AddFile(hardenPayloadDir+"/bin/go", []byte("go"), 0o755)
	// 通常file。
	filesystem.AddFile(hardenPayloadDir+"/VERSION", []byte("go1.25.0"), 0o644)
	return filesystem, injector
}

func hardenPayloadValue(t *testing.T) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(domain.RolePayload, hardenPayloadDir)
	if err != nil {
		t.Fatalf("NewPathValue: %v", err)
	}
	return value
}

// TestHardenPayloadNormalizesEveryEntry は種別ごとの正規化を固定する。
//
// docs/08-install-runtime.md §7手順5「Linuxはdirectory 0555、executable 0555、
// その他0444を基本とする」。
func TestHardenPayloadNormalizesEveryEntry(t *testing.T) {
	filesystem, _ := newHardenHarness(t)
	host := platformOf(t, "linux-amd64-glibc")

	if err := HardenPayload(filesystem, hardenPayloadValue(t), host); err != nil {
		t.Fatalf("HardenPayload = %v", err)
	}

	want := map[string]port.PermissionKind{
		// **payload root自身も対象にする。** 書込み可能なままだと、中身が
		// read onlyでもentryの追加・削除ができる。
		hardenPayloadDir:              port.PermissionDirectory,
		hardenPayloadDir + "/bin":     port.PermissionDirectory,
		hardenPayloadDir + "/bin/go":  port.PermissionExecutable,
		hardenPayloadDir + "/VERSION": port.PermissionRegular,
	}
	records := filesystem.Hardened()
	if len(records) != len(want) {
		t.Fatalf("正規化 = %d件, want %d件: %+v", len(records), len(want), records)
	}
	for _, record := range records {
		expected, ok := want[record.Path]
		if !ok {
			t.Errorf("想定外のentryを正規化した: %q", record.Path)
			continue
		}
		if record.Kind != expected {
			t.Errorf("%q の種別 = %s, want %s", record.Path, record.Kind, expected)
		}
	}

	// modeが実際に落ちていること。
	for path, kind := range want {
		info, err := filesystem.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q): %v", path, err)
		}
		wantPerm := fs.FileMode(0o444)
		if kind != port.PermissionRegular {
			wantPerm = 0o555
		}
		if got := info.Mode & fs.ModePerm; got != wantPerm {
			t.Errorf("%q のmode = %v, want %v", path, got, wantPerm)
		}
	}
}

// TestHardenPayloadRejectsSymlink はpayload内のsymlinkを拒否することを固定する。
//
// §6が展開時にsymlinkを拒否しており、payload内に存在しないはずである。
// 現れた場合は展開後に差し込まれたことを意味し、permissionを正規化して
// 先へ進むより失敗させるほうが安全である。
func TestHardenPayloadRejectsSymlink(t *testing.T) {
	tests := []struct {
		name string
		kind port.LinkKind
	}{
		{"symlink", port.LinkSymlink},
		// **junctionはdirectoryとして報告される。** `IsDir`だけを見ると
		// 普通のdirectoryとして正規化してしまい、payload外を指すlinkが
		// payload内に残る。`IsSymlink`（reparse pointを含む）で判定する
		// ことをこのcaseが固定する。
		{"junction（reparse point）", port.LinkJunction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filesystem, _ := newHardenHarness(t)
			filesystem.AddLink(hardenPayloadDir+"/link", test.kind, "/etc/passwd")

			err := HardenPayload(filesystem, hardenPayloadValue(t),
				platformOf(t, "linux-amd64-glibc"))
			if err == nil {
				t.Fatal("payload内のlinkが通った")
			}
		})
	}
}

// TestHardenPayloadReportsFailure は失敗注入を固定する。
func TestHardenPayloadReportsFailure(t *testing.T) {
	filesystem, injector := newHardenHarness(t)
	injector.Fail(fake.OpHarden, 1, 1, errors.New("注入した正規化失敗"))

	err := HardenPayload(filesystem, hardenPayloadValue(t),
		platformOf(t, "linux-amd64-glibc"))
	if err == nil {
		t.Fatal("正規化失敗で成功した")
	}
}

// TestHardenPayloadRejectsInvalidInput は前提違反を拒否することを固定する。
func TestHardenPayloadRejectsInvalidInput(t *testing.T) {
	filesystem, _ := newHardenHarness(t)
	host := platformOf(t, "linux-amd64-glibc")

	if err := HardenPayload(nil, hardenPayloadValue(t), host); err == nil {
		t.Error("FileSystem無しで成功した")
	}
	if err := HardenPayload(filesystem, domain.PathValue{}, host); err == nil {
		t.Error("payload未設定で成功した")
	}
	// roleがpayloadでないpathを正規化しない。誤ってstate rootを渡されると、
	// 正本stateがread onlyになって以降の書込みが全部失敗する。
	wrongRole, err := domain.NewPathValue(domain.RoleStaging, hardenPayloadDir)
	if err != nil {
		t.Fatalf("NewPathValue: %v", err)
	}
	if hardenErr := HardenPayload(filesystem, wrongRole, host); hardenErr == nil {
		t.Error("payload以外のroleが通った")
	}
	if hardenErr := HardenPayload(filesystem, hardenPayloadValue(t),
		domain.Platform{}); hardenErr == nil {
		t.Error("host未設定で成功した")
	}
}

// TestPermissionKindCountMatchesSpec は種別数を固定する。
func TestPermissionKindCountMatchesSpec(t *testing.T) {
	valid := 0
	for kind := port.PermissionKind(1); kind <= port.PermissionKind(10); kind++ {
		if kind.IsValid() {
			valid++
		}
	}
	if valid != port.PermissionKindCount {
		t.Errorf("有効な種別 = %d件, want %d件", valid, port.PermissionKindCount)
	}
	if port.PermissionKind(0).IsValid() {
		t.Error("zero値が有効になっている")
	}
}
