package fake

import (
	"errors"
	"reflect"
	"testing"
)

func TestInjectorNoFaultReturnsNil(t *testing.T) {
	inj := NewInjector()
	for i := 0; i < 3; i++ {
		if err := inj.Check("op"); err != nil {
			t.Fatalf("Check(%d) = %v, want nil", i, err)
		}
	}
	if got := inj.Calls("op"); got != 3 {
		t.Fatalf("Calls = %d, want 3", got)
	}
}

func TestInjectorFailOnce(t *testing.T) {
	inj := NewInjector()
	want := errors.New("boom")
	inj.FailOnce("op", want)

	if err := inj.Check("op"); !errors.Is(err, want) {
		t.Fatalf("1回目 = %v, want %v", err, want)
	}
	if err := inj.Check("op"); err != nil {
		t.Fatalf("2回目 = %v, want nil", err)
	}
}

func TestInjectorFailAfterSkip(t *testing.T) {
	inj := NewInjector()
	want := errors.New("disk full")
	// 2回成功させてから3回目で失敗させる。
	inj.Fail("op", 2, 1, want)

	if err := inj.Check("op"); err != nil {
		t.Fatalf("1回目 = %v, want nil", err)
	}
	if err := inj.Check("op"); err != nil {
		t.Fatalf("2回目 = %v, want nil", err)
	}
	if err := inj.Check("op"); !errors.Is(err, want) {
		t.Fatalf("3回目 = %v, want %v", err, want)
	}
	if err := inj.Check("op"); err != nil {
		t.Fatalf("4回目 = %v, want nil", err)
	}
}

func TestInjectorUnlimitedFailure(t *testing.T) {
	inj := NewInjector()
	want := errors.New("always")
	inj.Fail("op", 0, 0, want)

	for i := 0; i < 5; i++ {
		if err := inj.Check("op"); !errors.Is(err, want) {
			t.Fatalf("Check(%d) = %v, want %v", i, err, want)
		}
	}
}

func TestInjectorIsolatesOperations(t *testing.T) {
	inj := NewInjector()
	want := errors.New("only-a")
	inj.Fail("a", 0, 0, want)

	if err := inj.Check("b"); err != nil {
		t.Fatalf("b = %v, want nil", err)
	}
	if err := inj.Check("a"); !errors.Is(err, want) {
		t.Fatalf("a = %v, want %v", err, want)
	}
}

func TestInjectorOperationsSorted(t *testing.T) {
	inj := NewInjector()
	_ = inj.Check("zeta")
	_ = inj.Check("alpha")
	_ = inj.Check("mid")

	want := []string{"alpha", "mid", "zeta"}
	if got := inj.Operations(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Operations = %v, want %v", got, want)
	}
}

func TestInjectorPendingReportsUnconsumed(t *testing.T) {
	inj := NewInjector()
	inj.FailOnce("never-called", errors.New("unused"))

	if got := inj.Pending(); len(got) != 1 {
		t.Fatalf("Pending = %v, want 1件", got)
	}
	_ = inj.Check("never-called")
	if got := inj.Pending(); len(got) != 0 {
		t.Fatalf("消化後のPending = %v, want 0件", got)
	}
}

func TestInjectorFailRejectsNilError(t *testing.T) {
	inj := NewInjector()
	defer func() {
		if recover() == nil {
			t.Fatal("nil errorでpanicしなかった")
		}
	}()
	inj.Fail("op", 0, 1, nil)
}
