// Package ciprobe はP0-02のCI検証専用の一時packageである。
//
// go vet、govulncheck、go test、coverageが両OSで実際に走ることを一度だけ
// CIで確認するために置く。確認後、同じPR内で削除する。
package ciprobe

// Add は2つの整数を足す。
func Add(a, b int) int {
	return a + b
}
