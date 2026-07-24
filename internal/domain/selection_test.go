package domain

import (
	"errors"
	"testing"
)

func TestNewInstallID(t *testing.T) {
	valid := "00000000-0000-4000-8000-000000000001"
	id, err := NewInstallID(valid)
	if err != nil {
		t.Fatalf("NewInstallID: %v", err)
	}
	if id.String() != valid || id.IsZero() {
		t.Errorf("InstallID の値が不正: %q", id.String())
	}
	for name, s := range map[string]string{"empty": "", "space": "a b", "control": "a\x01b"} {
		if _, err := NewInstallID(s); !errors.Is(err, ErrInvalidSelection) {
			t.Errorf("NewInstallID(%s) は ErrInvalidSelection: %v", name, err)
		}
	}
}

func TestNewSelection(t *testing.T) {
	id := mustToolID(t, "node")
	ver := mustVersion(t, "22.18.0")
	iid, _ := NewInstallID("00000000-0000-4000-8000-000000000001")
	sel, err := NewSelection(id, ver, "", iid)
	if err != nil {
		t.Fatalf("NewSelection: %v", err)
	}
	if sel.Variant() != DefaultVariant {
		t.Errorf("variant 空は既定へ正規化: %q", sel.Variant())
	}
	if sel.ToolID() != id || sel.Version() != ver || sel.InstallID() != iid {
		t.Error("Selection の要素が一致しない")
	}
}

func TestNewSelection_Invalid(t *testing.T) {
	id := mustToolID(t, "node")
	ver := mustVersion(t, "22.18.0")
	iid, _ := NewInstallID("00000000-0000-4000-8000-000000000001")

	if _, err := NewSelection(ToolID{}, ver, "", iid); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("tool ID 欠落は ErrInvalidSelection: %v", err)
	}
	if _, err := NewSelection(id, Version{}, "", iid); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("version 欠落は ErrInvalidSelection: %v", err)
	}
	if _, err := NewSelection(id, ver, "", InstallID{}); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("install ID 欠落は ErrInvalidSelection: %v", err)
	}
	if _, err := NewSelection(id, ver, "a/b", iid); !errors.Is(err, ErrInvalidVariant) {
		t.Errorf("不正 variant は ErrInvalidVariant: %v", err)
	}
}

func TestNewEffectiveSelection(t *testing.T) {
	id := mustToolID(t, "go")
	ver := mustVersion(t, "1.26.5")

	// 選択あり（user/project/explicit）は version 必須。
	for _, o := range []Origin{OriginUser, OriginProject, OriginExplicit} {
		e, err := NewEffectiveSelection(id, o, ver, "")
		if err != nil {
			t.Fatalf("NewEffectiveSelection(%s): %v", o, err)
		}
		if !e.IsSelected() {
			t.Errorf("origin=%s は IsSelected=true であるべき", o)
		}
		if e.Version() != ver {
			t.Errorf("origin=%s の version 不一致", o)
		}
	}

	// 未選択/無効化は version を持たない。
	for _, o := range []Origin{OriginNone, OriginDisabled} {
		e, err := NewEffectiveSelection(id, o, Version{}, "")
		if err != nil {
			t.Fatalf("NewEffectiveSelection(%s): %v", o, err)
		}
		if e.IsSelected() {
			t.Errorf("origin=%s は IsSelected=false であるべき", o)
		}
	}
}

func TestNewEffectiveSelection_Invalid(t *testing.T) {
	id := mustToolID(t, "go")
	ver := mustVersion(t, "1.26.5")

	// 選択ありなのに version 欠落。
	if _, err := NewEffectiveSelection(id, OriginUser, Version{}, ""); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("origin=user かつ version 欠落は ErrInvalidSelection: %v", err)
	}
	// 未選択なのに version あり。
	if _, err := NewEffectiveSelection(id, OriginNone, ver, ""); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("origin=none かつ version ありは ErrInvalidSelection: %v", err)
	}
	// 未知 origin。
	if _, err := NewEffectiveSelection(id, Origin("bogus"), Version{}, ""); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("未知 origin は ErrInvalidSelection: %v", err)
	}
}

func TestNewOrigin(t *testing.T) {
	for _, s := range []string{"none", "user", "project", "explicit", "disabled"} {
		if _, err := NewOrigin(s); err != nil {
			t.Errorf("NewOrigin(%q): %v", s, err)
		}
	}
	if _, err := NewOrigin("bogus"); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("NewOrigin(bogus) は ErrInvalidSelection: %v", err)
	}
}
