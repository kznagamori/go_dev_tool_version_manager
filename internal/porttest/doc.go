// Package porttest は 02章5節の抽象 port の fake/stub 実装をテスト用に提供する。
//
// FakeClock・FakeIDGenerator・RecordingSink 等は決定的な振る舞いを持ち、その他の OS
// 依存 port は ErrNotImplemented を返す stub とする。各テストは必要な port だけ本物の
// fake に差し替える。本 package は _test ではなく通常 package だが、production からは
// import されない（internal かつ test 専用の用途）。
package porttest
