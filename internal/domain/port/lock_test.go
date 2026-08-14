package port

import (
	"strings"
	"testing"
)

// TestLockClassOrderMatchesSpec はdocs/02-architecture.md §12の6分類と順序を固定する。
//
// 数値の大小がそのまま取得順であるため、定数の並びが仕様の並びと一致していなければ
// lock順序の検査全体が意味を失う。
func TestLockClassOrderMatchesSpec(t *testing.T) {
	want := []struct {
		class LockClass
		name  string
	}{
		{ClassState, "state"},
		{ClassCatalog, "catalog"},
		{ClassInstall, "install"},
		{ClassStorage, "storage"},
		{ClassSetup, "setup"},
		{ClassShim, "shim"},
	}
	if len(want) != LockClassCount {
		t.Fatalf("LockClassCountは%dでなければならない（%d）", len(want), LockClassCount)
	}
	for index, entry := range want {
		if entry.class.String() != entry.name {
			t.Errorf("class %d の名前 = %q, want %q", entry.class, entry.class.String(), entry.name)
		}
		if !entry.class.IsValid() {
			t.Errorf("class %q がIsValidでない", entry.name)
		}
		// 数値が§12の並び順（1始まり）と一致する。
		if int(entry.class) != index+1 {
			t.Errorf("class %q の値 = %d, want %d", entry.name, entry.class, index+1)
		}
		parsed, err := ParseLockClass(entry.name)
		if err != nil || parsed != entry.class {
			t.Errorf("ParseLockClass(%q) = %v, %v", entry.name, parsed, err)
		}
	}
	if _, err := ParseLockClass("payload"); err == nil {
		t.Error("未定義のroleが通った")
	}
	if LockClass(0).IsValid() || LockClass(99).IsValid() {
		t.Error("範囲外のclassがIsValidになった")
	}
}

// TestLockKeyRequiresQualifierPerClass は§12のclassごとの対象数を固定する。
//
// catalog/install/storageは同一class内に複数対象があり、state/setup/shimは1つである。
func TestLockKeyRequiresQualifierPerClass(t *testing.T) {
	single := []LockClass{ClassState, ClassSetup, ClassShim}
	for _, class := range single {
		key, err := LockKey(class, nil)
		if err != nil {
			t.Fatalf("class %q のqualifier無しが落ちた: %v", class, err)
		}
		if key != class.String() {
			t.Errorf("key = %q, want %q", key, class.String())
		}
		if _, err := LockKey(class, []string{"node"}); err == nil {
			t.Errorf("class %q が余分なqualifierを受理した", class)
		}
	}

	multi := []LockClass{ClassCatalog, ClassInstall, ClassStorage}
	for _, class := range multi {
		if _, err := LockKey(class, nil); err == nil {
			t.Errorf("class %q がqualifier無しを受理した", class)
		}
		if _, err := LockKey(class, []string{"node"}); err != nil {
			t.Errorf("class %q のqualifierが落ちた: %v", class, err)
		}
	}
}

// TestLockKeySeparatorAvoidsAmbiguity は区切りに`-`を使わない判断を固定する。
//
// tool ID、platform ID、storage IDはkebab-caseで`-`を含み、semverのprereleaseも
// `-`を含む。`-`区切りだと tool `a-b`＋version `c` と tool `a`＋version `b-c` が
// 同じkeyになり、別の対象が同じlockを共有してしまう。
func TestLockKeySeparatorAvoidsAmbiguity(t *testing.T) {
	left, err := LockKey(ClassInstall, []string{"a-b", "c", "linux-amd64-glibc"})
	if err != nil {
		t.Fatalf("LockKey = %v", err)
	}
	right, err := LockKey(ClassInstall, []string{"a", "b-c", "linux-amd64-glibc"})
	if err != nil {
		t.Fatalf("LockKey = %v", err)
	}
	if left == right {
		t.Fatalf("別のtool/versionが同じlock key %q になった", left)
	}
	// semverのprereleaseを含むversionも区別できる。
	pre, err := LockKey(ClassInstall, []string{"node", "1.0.0-rc.1", "linux-amd64-glibc"})
	if err != nil {
		t.Fatalf("LockKey = %v", err)
	}
	if pre == left || pre == right {
		t.Error("prerelease versionのkeyが衝突した")
	}
	// SplitLockKeyで元の構成要素へ戻せる。
	parts := SplitLockKey(pre)
	if len(parts) != 4 || parts[0] != "install" || parts[2] != "1.0.0-rc.1" {
		t.Errorf("SplitLockKey(%q) = %v", pre, parts)
	}
}

// TestLockKeyRejectsUnsafeQualifier はlock fileがlock directoryの外へ出ないことを
// 固定する（docs/04-storage-and-data.md §6）。
func TestLockKeyRejectsUnsafeQualifier(t *testing.T) {
	rejects := []struct {
		name      string
		qualifier string
	}{
		{"空", ""},
		{"区切りを含む", "no~de"},
		{"slash", "a/b"},
		{"backslash", `a\b`},
		{"NUL", "a\x00b"},
		{"相対参照", ".."},
		{"カレント", "."},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LockKey(ClassCatalog, []string{test.qualifier}); err == nil {
				t.Errorf("qualifier %q が通った", test.qualifier)
			}
		})
	}
}

// TestCompareLocksOrdersByClassThenKey は§12の順序規則を固定する。
func TestCompareLocksOrdersByClassThenKey(t *testing.T) {
	// classが違えばclassの数値順。
	if CompareLocks(ClassState, "state", ClassShim, "shim") >= 0 {
		t.Error("stateがshimより後になった")
	}
	if CompareLocks(ClassShim, "shim", ClassState, "state") <= 0 {
		t.Error("shimがstateより先になった")
	}
	// 同classならkeyのbyte順。§12のToolID順に対応する。
	nodeKey, _ := LockKey(ClassCatalog, []string{"node"})
	pythonKey, _ := LockKey(ClassCatalog, []string{"python"})
	if CompareLocks(ClassCatalog, nodeKey, ClassCatalog, pythonKey) >= 0 {
		t.Errorf("catalog %q が %q より後になった", nodeKey, pythonKey)
	}
	if CompareLocks(ClassCatalog, nodeKey, ClassCatalog, nodeKey) != 0 {
		t.Error("同一keyが0にならない")
	}
	// classの数値順がkeyのbyte順より優先する。`catalog`は`state`よりbyte順で
	// 先だが、§12ではstateが先である。
	if !strings.HasPrefix(nodeKey, "catalog") {
		t.Fatalf("前提が崩れている: %q", nodeKey)
	}
	if CompareLocks(ClassState, "state", ClassCatalog, nodeKey) >= 0 {
		t.Error("classよりkeyのbyte順が優先されている")
	}
}
