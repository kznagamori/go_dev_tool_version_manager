package port

import (
	"fmt"
	"strings"
)

// lockQualifierSeparator はlock keyのclassとqualifierを区切る文字である。
//
// `-`ではなく`~`を使う。tool ID、platform ID、storage IDはkebab-caseで`-`を
// 含み、versionもsemverのprerelease（`1.0.0-rc.1`）で`-`を含む。`-`区切りだと
// tool `a-b`＋version `c` と tool `a`＋version `b-c` が同じkeyになり、別の対象が
// 同じlockを共有してしまう。どの構成要素にも現れない文字で区切る。
const lockQualifierSeparator = "~"

// LockKey は`locks/<role>.lock`のrole部分を組み立てる。
//
// docs/04-storage-and-data.md §19はlockの保存先を`locks/<role>.lock`と定めるが、
// classごとに複数のlockを持つ場合（§12のcatalog/install/storage）のrole名は
// 定めていない。同一classの対象を別fileへ分けるには一意なrole名が要るため、
// `<class>~<qualifier>~...`で組み立てる。
//
// qualifierの各要素はpath componentとして安全でなければならない。区切りや
// 相対参照が入ると、lock fileがlock directoryの外へ出る
// （docs/04-storage-and-data.md §6）。
func LockKey(class LockClass, qualifier []string) (string, error) {
	if !class.IsValid() {
		return "", fmt.Errorf("port: 未定義のlock class %d", class)
	}
	// state、setup、shimは対象が1つでqualifierを取らない。余分なqualifierを
	// 黙って無視すると、別の対象のつもりで取ったlockが同じfileを指す。
	if !classTakesQualifier(class) && len(qualifier) > 0 {
		return "", fmt.Errorf("port: lock class %q はqualifierを取らない", class)
	}
	if classTakesQualifier(class) && len(qualifier) == 0 {
		return "", fmt.Errorf("port: lock class %q はqualifierが必要", class)
	}
	parts := make([]string, 0, len(qualifier)+1)
	parts = append(parts, class.String())
	for index, part := range qualifier {
		if err := validateLockQualifier(index, part); err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, lockQualifierSeparator), nil
}

// classTakesQualifier は同一class内に複数対象を持つclassかを返す（§12）。
func classTakesQualifier(class LockClass) bool {
	switch class {
	case ClassCatalog, ClassInstall, ClassStorage:
		return true
	default:
		return false
	}
}

func validateLockQualifier(index int, part string) error {
	switch {
	case part == "":
		return fmt.Errorf("port: lock qualifier[%d]が空", index)
	case strings.Contains(part, lockQualifierSeparator):
		return fmt.Errorf(
			"port: lock qualifier[%d] %q に区切り %q が含まれる", index, part, lockQualifierSeparator)
	case strings.ContainsAny(part, `/\`):
		return fmt.Errorf("port: lock qualifier[%d] %q にpath区切りが含まれる", index, part)
	case strings.ContainsRune(part, 0):
		return fmt.Errorf("port: lock qualifier[%d]にNULが含まれる", index)
	case part == "." || part == "..":
		return fmt.Errorf("port: lock qualifier[%d] %q は相対参照である", index, part)
	}
	return nil
}

// SplitLockKey はlock keyをclass名とqualifierへ分解する。
//
// lock fileを読む側（診断とdoctor）が、[LockKey]と同じ区切りを自前で持たずに
// 分解できるようにする。区切りを2か所で定義すると、片方だけ変えたときに
// 検出できない。
func SplitLockKey(key string) []string {
	return strings.Split(key, lockQualifierSeparator)
}

// CompareLocks は§12のlock取得順で2つのlockを比較する。
//
// classが違えばclassの数値順、同じならkeyのASCII byte順とする。§12が
// catalogをToolID順、installをToolID/version/platform順、storageをToolID/
// storage ID順と定めており、[LockKey]がその順にqualifierを連結するため、
// key のbyte順が仕様の順序と一致する。
//
// 戻り値は左が先なら負、同じなら0、右が先なら正である。
func CompareLocks(leftClass LockClass, leftKey string, rightClass LockClass, rightKey string) int {
	if leftClass != rightClass {
		if leftClass < rightClass {
			return -1
		}
		return 1
	}
	return strings.Compare(leftKey, rightKey)
}
