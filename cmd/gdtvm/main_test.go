package main

import (
	"bytes"
	"strings"
	"testing"
)

// 骨格段階のCLIが成功を返さないことを確認する。
// 未実装のまま exit 0 を返すと、CIやscriptが実装済みと誤認する。
func TestRunReportsNotImplemented(t *testing.T) {
	var stderr bytes.Buffer

	code := run(&stderr)

	if code == 0 {
		t.Fatalf("run = %d, want 非0", code)
	}
	if !strings.Contains(stderr.String(), "P8-04") {
		t.Fatalf("stderr に実装taskの参照が無い: %q", stderr.String())
	}
}
