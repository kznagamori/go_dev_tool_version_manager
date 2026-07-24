// Package definition は tool 定義 TOML の解析、schema 検証、テンプレート評価を担う
// （06章、15章）。
//
// version 発見 adapter、artifact 選択、condition 評価、install step DAG、runtime/
// validation の解釈を提供する。tool 固有の Go 分岐を持たず、標準定義もローカル定義も
// 同じ parser/validator/planner で処理する。
package definition
