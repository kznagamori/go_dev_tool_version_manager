package ports

// IDGenerator は operation ID や install ID を生成する抽象 port である（10章2節）。
// テストでは決定的な連番/固定値へ差し替える。
type IDGenerator interface {
	// NewID は衝突しない新しい ID 文字列を返す（UUID 形式を推奨、14章2節）。
	NewID() string
}
