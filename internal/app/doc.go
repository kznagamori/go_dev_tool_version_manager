// Package app は Application Service（ユースケースの公開窓口）を提供する。
//
// 各 operation は要求値・結果値・型付きエラーだけを境界に出し、context による
// cancel と operation ID を扱う（10章）。CLI/TOML/Wails/OS の具体型を境界へ露出
// させず、進捗は EventSink、確認は Prompt/Approval provider へ委ねる。実処理は
// domain とインフラ port へ委譲し、本パッケージは要求検証とトランザクション境界を担う。
package app
