package domain

import "fmt"

// idMaxLen は各種 ID 文字列の上限長である。UUID（36 文字）に余裕を持たせる。
const idMaxLen = 128

// validateIDString は ID 文字列が非空・ASCII 印字可能（空白・制御文字を含まない）・
// 上限長以内であることを検査する。14章2節は operation/install/approval ID に UUID
// 形式を推奨するが必須にしないため、形式は UUID に限定しない。
func validateIDString(kind, s string) error {
	if s == "" {
		return fmt.Errorf("%s が空", kind)
	}
	if len(s) > idMaxLen {
		return fmt.Errorf("%s が長すぎる（%d 文字、上限 %d）", kind, len(s), idMaxLen)
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x21 || c > 0x7e {
			return fmt.Errorf("%s に ASCII 印字可能文字以外を含む", kind)
		}
	}
	return nil
}

// InstallID は導入物を識別する ID である（14章2節、UUID 形式推奨）。receipt と
// selection の install_id はこの値で一致しなければならない。
type InstallID struct {
	value string
}

// NewInstallID は文字列を検証して InstallID を生成する。空・非 ASCII・過長は
// ErrInvalidSelection を包んで返す。
func NewInstallID(s string) (InstallID, error) {
	if err := validateIDString("install ID", s); err != nil {
		return InstallID{}, fmt.Errorf("%w: %v", ErrInvalidSelection, err)
	}
	return InstallID{value: s}, nil
}

// String は install ID 文字列を返す。
func (i InstallID) String() string { return i.value }

// IsZero はゼロ値（未設定）の InstallID かどうかを返す。
func (i InstallID) IsZero() bool { return i.value == "" }
