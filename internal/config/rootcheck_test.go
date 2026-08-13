package config

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// rootFS はentryごとの属性を持つtest用FileSystemである。
//
// port.FileSystemを埋め込んで未使用methodを補う。CheckRootはStatとRealPathしか
// 使わないため、他のmethodが呼ばれたらnil panicで気付ける。
type rootFS struct {
	port.FileSystem
	entries  map[string]port.FileInfo
	real     map[string]string
	statFail map[string]error
	realFail map[string]error
}

func newRootFS() *rootFS {
	return &rootFS{
		entries:  make(map[string]port.FileInfo),
		real:     make(map[string]string),
		statFail: make(map[string]error),
		realFail: make(map[string]error),
	}
}

// dir はowner自身だけが書けるdirectoryを登録する。
func (r *rootFS) dir(path string) *rootFS {
	r.entries[path] = port.FileInfo{Mode: fs.ModeDir | 0o755, IsDir: true}
	return r
}

func (r *rootFS) withMode(path string, perm fs.FileMode) *rootFS {
	r.entries[path] = port.FileInfo{Mode: fs.ModeDir | perm, IsDir: true}
	return r
}

func (r *rootFS) file(path string) *rootFS {
	r.entries[path] = port.FileInfo{Mode: 0o644}
	return r
}

func (r *rootFS) link(path string) *rootFS {
	r.entries[path] = port.FileInfo{Mode: fs.ModeDir | 0o755, IsDir: true, IsSymlink: true}
	return r
}

func (r *rootFS) Stat(path string) (port.FileInfo, error) {
	if err, ok := r.statFail[path]; ok {
		return port.FileInfo{}, err
	}
	if info, ok := r.entries[path]; ok {
		return info, nil
	}
	return port.FileInfo{}, fs.ErrNotExist
}

func (r *rootFS) RealPath(path string) (string, error) {
	if err, ok := r.realFail[path]; ok {
		return "", err
	}
	if resolved, ok := r.real[path]; ok {
		return resolved, nil
	}
	return path, nil
}

// ownerLookup はpathごとのownerを持つtest用UserLookupである。
type ownerLookup struct {
	port.UserLookup
	self    string
	owners  map[string]string
	failing map[string]error
}

func newOwnerLookup(self string) *ownerLookup {
	return &ownerLookup{self: self, owners: make(map[string]string), failing: make(map[string]error)}
}

func (o *ownerLookup) OwnerOf(path string) (string, error) {
	if err, ok := o.failing[path]; ok {
		return "", err
	}
	if owner, ok := o.owners[path]; ok {
		return owner, nil
	}
	return o.self, nil
}

const testUID = "1000"

func rootValue(t *testing.T, path string) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(domain.RoleDataRoot, path)
	if err != nil {
		t.Fatalf("NewPathValue = %v", err)
	}
	return value
}

func checkRequest(t *testing.T, path string, filesystem *rootFS, lookup *ownerLookup) RootCheckRequest {
	t.Helper()
	return RootCheckRequest{
		Root:       rootValue(t, path),
		Host:       linuxHost(t),
		User:       port.UserIdentity{Name: "alice", ID: testUID, Home: "/home/alice"},
		FileSystem: filesystem,
		UserLookup: lookup,
	}
}

func TestCheckRootAcceptsSafeExistingRoot(t *testing.T) {
	filesystem := newRootFS().dir("/home/alice/.local/share/gdtvm").
		dir("/home/alice/.local/share").dir("/home/alice/.local").dir("/home/alice").dir("/home")

	got, err := CheckRoot(checkRequest(t, "/home/alice/.local/share/gdtvm", filesystem, newOwnerLookup(testUID)))
	if err != nil {
		t.Fatalf("CheckRoot = %v, want nil", err)
	}
	if !got.Existed {
		t.Error("Existed = false, want true")
	}
	if got.Canonical.Path() != "/home/alice/.local/share/gdtvm" {
		t.Errorf("Canonical = %q", got.Canonical.Path())
	}
	if got.Canonical.Role() != domain.RoleDataRoot {
		t.Errorf("Role = %q", got.Canonical.Role())
	}
}

// TestCheckRootResolvesCanonicalPath はrealpathの結果を返すことを固定する。
//
// docs/04-storage-and-data.md §6のcontainment判定はcanonical pathで行うため、
// 解決前のpathを返すとsymlink経由の逸脱を見逃す。
func TestCheckRootResolvesCanonicalPath(t *testing.T) {
	filesystem := newRootFS().dir("/srv/link-target").dir("/srv").dir("/opt/gdtvm").dir("/opt")
	filesystem.real["/opt/gdtvm"] = "/srv/link-target"

	got, err := CheckRoot(checkRequest(t, "/opt/gdtvm", filesystem, newOwnerLookup(testUID)))
	if err != nil {
		t.Fatalf("CheckRoot = %v, want nil", err)
	}
	if got.Canonical.Path() != "/srv/link-target" {
		t.Errorf("Canonical = %q, want /srv/link-target", got.Canonical.Path())
	}
}

// TestCheckRootAllowsCreatableRoot は未作成rootを許すことを固定する。
//
// setupはrootを作るのが仕事であり、存在しないことを理由に拒否すると初回setupが
// できなくなる。
func TestCheckRootAllowsCreatableRoot(t *testing.T) {
	filesystem := newRootFS().dir("/home/alice").dir("/home")

	got, err := CheckRoot(checkRequest(t, "/home/alice/.local/share/gdtvm", filesystem, newOwnerLookup(testUID)))
	if err != nil {
		t.Fatalf("CheckRoot = %v, want nil", err)
	}
	if got.Existed {
		t.Error("Existed = true, want false")
	}
	if got.Canonical.Path() != "/home/alice/.local/share/gdtvm" {
		t.Errorf("Canonical = %q", got.Canonical.Path())
	}
}

// TestCheckRootRejectsUnsafeRoot はdocs/09-platform.md §2.3の拒否条件を固定する。
func TestCheckRootRejectsUnsafeRoot(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		host  domain.Platform
		build func() (*rootFS, *ownerLookup)
		want  domain.ErrorCode
	}{
		{
			"filesystem root（linux）", "/", linuxHost(t),
			func() (*rootFS, *ownerLookup) { return newRootFS().dir("/"), newOwnerLookup(testUID) },
			domain.CodePathUnsafe,
		},
		{
			"drive root（windows）", `C:\`, windowsHost(t),
			func() (*rootFS, *ownerLookup) { return newRootFS().dir(`C:\`), newOwnerLookup(testUID) },
			domain.CodePathUnsafe,
		},
		{
			"network share（windows UNC）", `\\server\share\gdtvm`, windowsHost(t),
			func() (*rootFS, *ownerLookup) { return newRootFS(), newOwnerLookup(testUID) },
			domain.CodePathUnsafe,
		},
		{
			"rootがsymlink/reparse point", "/opt/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				return newRootFS().link("/opt/gdtvm").dir("/opt"), newOwnerLookup(testUID)
			},
			domain.CodePathUnsafe,
		},
		{
			"rootがdirectoryでない", "/opt/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				return newRootFS().file("/opt/gdtvm").dir("/opt"), newOwnerLookup(testUID)
			},
			domain.CodePathUnsafe,
		},
		{
			"root自身が他user所有", "/opt/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				lookup := newOwnerLookup(testUID)
				lookup.owners["/opt/gdtvm"] = "0"
				return newRootFS().dir("/opt/gdtvm").dir("/opt"), lookup
			},
			domain.CodePermission,
		},
		{
			"親が他user所有", "/opt/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				lookup := newOwnerLookup(testUID)
				lookup.owners["/opt"] = "0"
				return newRootFS().dir("/opt/gdtvm").dir("/opt"), lookup
			},
			domain.CodePermission,
		},
		{
			"root自身がworld-writable", "/opt/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				return newRootFS().withMode("/opt/gdtvm", 0o777).dir("/opt"), newOwnerLookup(testUID)
			},
			domain.CodePermission,
		},
		{
			"root自身がgroup-writable", "/opt/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				return newRootFS().withMode("/opt/gdtvm", 0o775).dir("/opt"), newOwnerLookup(testUID)
			},
			domain.CodePermission,
		},
		{
			"親がworld-writable", "/tmp/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				return newRootFS().dir("/tmp/gdtvm").withMode("/tmp", 0o777), newOwnerLookup(testUID)
			},
			domain.CodePermission,
		},
		{
			"未作成rootの祖先が他user所有", "/opt/gdtvm/data", linuxHost(t),
			func() (*rootFS, *ownerLookup) {
				lookup := newOwnerLookup(testUID)
				lookup.owners["/opt"] = "0"
				return newRootFS().dir("/opt"), lookup
			},
			domain.CodePermission,
		},
		{
			"未作成rootに既存祖先が無い", "/nowhere/gdtvm", linuxHost(t),
			func() (*rootFS, *ownerLookup) { return newRootFS(), newOwnerLookup(testUID) },
			domain.CodePathUnsafe,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filesystem, lookup := test.build()
			request := checkRequest(t, test.path, filesystem, lookup)
			request.Host = test.host

			_, err := CheckRoot(request)
			if err == nil {
				t.Fatal("CheckRoot = nil, want error")
			}
			if err.Code != test.want {
				t.Errorf("Code = %s, want %s", err.Code, test.want)
			}
			// 個人pathを公開境界へ出さず、roleだけを伝える（docs/10-security.md §9.2）。
			if err.PathRole != domain.RoleDataRoot {
				t.Errorf("PathRole = %q, want %q", err.PathRole, domain.RoleDataRoot)
			}
			for _, param := range err.Parameters {
				if text, ok := param.Str(); ok && text == test.path {
					t.Error("parametersへ実pathが入っている")
				}
			}
			if validateErr := err.Validate(); validateErr != nil {
				t.Errorf("typed errorがValidateで落ちた: %v", validateErr)
			}
		})
	}
}

// TestCheckRootSkipsModeCheckOnWindows はWindowsでmode bitを見ないことを固定する。
//
// WindowsのACLは fs.FileMode へ写らないため、mode bitでの他user書込み判定は
// 成立しない。ACLの検査はport実装の責務である。
func TestCheckRootSkipsModeCheckOnWindows(t *testing.T) {
	filesystem := newRootFS().withMode(`D:\gdtvm`, 0o777).withMode(`D:\`, 0o777)
	request := checkRequest(t, `D:\gdtvm`, filesystem, newOwnerLookup(testUID))
	request.Host = windowsHost(t)

	if _, err := CheckRoot(request); err != nil {
		t.Fatalf("CheckRoot = %v, want nil", err)
	}

	// ownerが違えばWindowsでも拒否する。
	lookup := newOwnerLookup(testUID)
	lookup.owners[`D:\gdtvm`] = "S-1-5-18"
	request.UserLookup = lookup
	if _, err := CheckRoot(request); err == nil {
		t.Error("Windowsで他user所有のrootが通った")
	} else if err.Code != domain.CodePermission {
		t.Errorf("Code = %s, want %s", err.Code, domain.CodePermission)
	}
}

// TestCheckRootFailsOnPortError はport失敗を握りつぶさないことを固定する。
func TestCheckRootFailsOnPortError(t *testing.T) {
	sentinel := errors.New("permission denied")

	tests := []struct {
		name  string
		build func() (*rootFS, *ownerLookup)
	}{
		{
			"Stat失敗",
			func() (*rootFS, *ownerLookup) {
				filesystem := newRootFS().dir("/opt")
				filesystem.statFail["/opt/gdtvm"] = sentinel
				return filesystem, newOwnerLookup(testUID)
			},
		},
		{
			"RealPath失敗",
			func() (*rootFS, *ownerLookup) {
				filesystem := newRootFS().dir("/opt/gdtvm").dir("/opt")
				filesystem.realFail["/opt/gdtvm"] = sentinel
				return filesystem, newOwnerLookup(testUID)
			},
		},
		{
			"OwnerOf失敗",
			func() (*rootFS, *ownerLookup) {
				lookup := newOwnerLookup(testUID)
				lookup.failing["/opt/gdtvm"] = sentinel
				return newRootFS().dir("/opt/gdtvm").dir("/opt"), lookup
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filesystem, lookup := test.build()
			_, err := CheckRoot(checkRequest(t, "/opt/gdtvm", filesystem, lookup))
			if err == nil {
				t.Fatal("CheckRoot = nil, want error")
			}
			if err.Code != domain.CodeFilesystem {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeFilesystem)
			}
			if !errors.Is(err, sentinel) {
				t.Error("causeを辿れない")
			}
		})
	}
}

func TestCheckRootRejectsIncompleteRequest(t *testing.T) {
	filesystem := newRootFS().dir("/opt/gdtvm").dir("/opt")
	lookup := newOwnerLookup(testUID)

	tests := []struct {
		name   string
		mutate func(*RootCheckRequest)
	}{
		{"root未設定", func(r *RootCheckRequest) { r.Root = domain.PathValue{} }},
		{"rootのpathが空", func(r *RootCheckRequest) { r.Root = rootValue(t, "") }},
		{"host未設定", func(r *RootCheckRequest) { r.Host = domain.Platform{} }},
		{"FileSystem未設定", func(r *RootCheckRequest) { r.FileSystem = nil }},
		{"UserLookup未設定", func(r *RootCheckRequest) { r.UserLookup = nil }},
		{"現在userのIDが空", func(r *RootCheckRequest) { r.User = port.UserIdentity{Name: "alice"} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := checkRequest(t, "/opt/gdtvm", filesystem, lookup)
			test.mutate(&request)

			_, err := CheckRoot(request)
			if err == nil {
				t.Fatal("CheckRoot = nil, want error")
			}
			if err.Code != domain.CodeUsage {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeUsage)
			}
		})
	}
}
