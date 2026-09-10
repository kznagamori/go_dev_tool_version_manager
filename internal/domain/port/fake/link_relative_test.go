package fake

import "testing"

// TestCreateSymlinkStoresRelativeTarget はfakeがport契約どおり相対形で保存する
// ことを固定する。
//
// **productionと保存値が違うと、保存値を読み返す呼出し側のtestが意味を失う。**
// docs/09-platform.md §5.1はcurrentとshimを「relative symlink」と定めており、
// 呼出し側は保存値がrelativeである前提で一致判定を書く。fakeがabsoluteを返すと、
// その判定はfake上で常に不一致になり、冪等性の検査が通らない。
func TestCreateSymlinkStoresRelativeTarget(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		linkPath string
		target   string
		relative bool
		want     string
	}{
		"親へ上がる":       {linkPath: "/root/shims/go", target: "/root/gdtvm", relative: true, want: "../gdtvm"},
		"同じdirectory": {linkPath: "/root/a", target: "/root/b", relative: true, want: "b"},
		"深いtarget":    {linkPath: "/root/shims/go", target: "/root/tools/go/1.25.0", relative: true, want: "../tools/go/1.25.0"},
		// relative=falseなら保存値はabsoluteのままである。
		"absolute指定": {linkPath: "/root/shims/go", target: "/root/gdtvm", relative: false, want: "/root/gdtvm"},
		// 既にrelativeなtargetはfixture記述の便宜としてそのまま保持する。
		"相対target": {linkPath: "/dist/current", target: "1.0.0", relative: true, want: "1.0.0"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fsys := NewFileSystem(NewInjector())
			links := NewLinkManager(fsys)
			if err := links.CreateSymlink(tc.linkPath, tc.target, tc.relative); err != nil {
				t.Fatalf("CreateSymlink: %v", err)
			}
			got, err := links.ReadLink(tc.linkPath)
			if err != nil {
				t.Fatalf("ReadLink: %v", err)
			}
			if got != tc.want {
				t.Errorf("target = %q, want %q", got, tc.want)
			}
		})
	}
}
