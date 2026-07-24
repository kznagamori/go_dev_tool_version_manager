package ports

// Locale は OS ロケールと端末能力を表す（02章5節, 05章3.1節）。Language は解決済みの
// "ja" または "en" とし、色使用可否は gdtvm 自身の terminal 判定に基づく。
type Locale struct {
	// Language は解決済み表示言語（"ja" または "en"）。
	Language string
	// IsTTY は人向け出力先が terminal かどうか。
	IsTTY bool
	// ColorCapable は色出力が可能かどうか（--json/redirect/pipe では false）。
	ColorCapable bool
}

// LocaleProvider は OS ロケールと端末能力を検出する抽象 port である（02章5節）。
type LocaleProvider interface {
	// Detect は現在の Locale を返す。
	Detect() Locale
}
