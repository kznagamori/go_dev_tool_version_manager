package store

import (
	"encoding/json"
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// EncodeLogLine は1件のlog recordをJSON Lines 1行へ変換する（§18）。
//
// 戻り値は末尾LFを含む1行である。呼出し側はこれを追記するだけでよい。
//
// mask後だけserializeし、credential/file content/registry rawを入れない（§18）。
// maskはlog recordを作る側の責務であり、ここではscalarのgrammarと件数だけを
// 検査する。値の中身を見て秘密を判定する経路を作ると、判定漏れが黙って通る。
func EncodeLogLine(record port.LogRecord) ([]byte, *domain.Error) {
	line, err := buildLogLine(record)
	if err != nil {
		return nil, typedError(domain.CodeInternal, "log.record_invalid", domain.RoleLog, err)
	}
	data, encodeErr := encodeJSON(line)
	if encodeErr != nil {
		return nil, typedError(domain.CodeInternal, "log.record_invalid", domain.RoleLog, encodeErr)
	}
	if len(data) > LogLineMaxBytes {
		return nil, typedError(domain.CodeInternal, "log.record_invalid", domain.RoleLog,
			fmt.Errorf("log 1行が%d byteを超える（%d byte）", LogLineMaxBytes, len(data)))
	}
	return data, nil
}

// logLine は§18のexact key集合である。
//
// encodeとdecodeで同じ型を使い、書けるkeyと読めるkeyがずれないようにする。
type logLine struct {
	Schema       int64          `json:"schema"`
	Time         string         `json:"time"`
	Level        string         `json:"level"`
	InvocationID string         `json:"invocation_id"`
	OperationID  string         `json:"operation_id"`
	Component    string         `json:"component"`
	MessageID    string         `json:"message_id"`
	Fields       map[string]any `json:"fields"`
}

func buildLogLine(record port.LogRecord) (logLine, error) {
	var line logLine
	// 不変条件は[port.LogRecord.Validate]が正本である。codec側で同じ検査を
	// 書き直すと、片方だけが緩む余地ができる。
	if err := record.Validate(); err != nil {
		return line, err
	}
	// operation_idは変更transactionだけが持つ。読取りcommandのlogでは空になる。
	operationID := ""
	if !record.Operation.IsZero() {
		operationID = record.Operation.String()
	}
	fields, err := encodeScalarMap(record.Fields, port.LogFieldsMax)
	if err != nil {
		return line, err
	}
	return logLine{
		Schema: SchemaVersion, Time: formatTimestamp(record.Time),
		Level: string(record.Level), InvocationID: record.Invocation.String(),
		OperationID: operationID, Component: record.Component,
		MessageID: record.MessageID.String(), Fields: fields,
	}, nil
}

// encodeScalarMap はscalar parameter mapをJSON値へ変換する（§7・§18）。
//
// keyのgrammarと件数上限を検査し、値をstring/bool/integer/nullへ限る。
// nested objectやarrayを許すと、log行の形が予測できなくなる。
func encodeScalarMap(parameters domain.Parameters, max int) (map[string]any, error) {
	if err := parameters.ValidateWithLimit(max); err != nil {
		return nil, err
	}
	encoded := make(map[string]any, len(parameters))
	for _, key := range parameters.SortedKeys() {
		value, err := scalarToJSON(parameters[key])
		if err != nil {
			return nil, fmt.Errorf("fields[%q]: %w", key, err)
		}
		encoded[key] = value
	}
	return encoded, nil
}

func scalarToJSON(scalar domain.Scalar) (any, error) {
	switch scalar.Kind() {
	case domain.ScalarNull:
		return nil, nil
	case domain.ScalarString:
		text, _ := scalar.Str()
		return text, nil
	case domain.ScalarBool:
		value, _ := scalar.Bool()
		return value, nil
	case domain.ScalarInt:
		value, _ := scalar.Int()
		if _, err := requireNonNegativeOrNegativeSafe(value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("未知のscalar kind %d", scalar.Kind())
	}
}

// requireNonNegativeOrNegativeSafe はintegerがJSON安全範囲であることを確かめる。
//
// §7の`byte count/revision`は非負だが、scalar parameterには差分のような負値も
// ありうる。符号は制限せず、絶対値が2^53-1以内であることだけを求める。
func requireNonNegativeOrNegativeSafe(value int64) (int64, error) {
	if value > JSONMaxSafeInteger || value < -JSONMaxSafeInteger {
		return 0, fmt.Errorf("integerが2^53-1の範囲を超える（%d）", value)
	}
	return value, nil
}

// DecodeLogLine はJSON Lines 1行をtyped recordへ戻す（§18）。
//
// doctorとtestがlogを読み直すために使う。書けない形のrecordを読めてしまうと、
// 出力側の検査が空振りするため、decodeもencodeと同じ制約で拒否する。
func DecodeLogLine(data []byte) (port.LogRecord, *domain.Error) {
	if len(data) > LogLineMaxBytes {
		return port.LogRecord{}, stateError("log.line_invalid", domain.RoleLog,
			fmt.Errorf("log 1行が%d byteを超える（%d byte）", LogLineMaxBytes, len(data)))
	}
	var line logLine
	if err := decodeJSON(data, &line); err != nil {
		return port.LogRecord{}, stateError("log.line_invalid", domain.RoleLog, err)
	}
	record, err := buildLogRecord(line)
	if err != nil {
		return port.LogRecord{}, stateError("log.line_invalid", domain.RoleLog, err)
	}
	return record, nil
}

func buildLogRecord(line logLine) (port.LogRecord, error) {
	var record port.LogRecord
	if line.Schema != SchemaVersion {
		return record, fmt.Errorf("schemaは%dだけを読める（%d）", SchemaVersion, line.Schema)
	}
	timestamp, err := parseTimestamp("time", line.Time)
	if err != nil {
		return record, err
	}
	level, err := port.ParseLogLevel(line.Level)
	if err != nil {
		return record, err
	}
	invocationID, idErr := domain.ParseInvocationID(line.InvocationID)
	if idErr != nil {
		return record, fmt.Errorf("invocation_id: %w", idErr)
	}
	var operationID domain.OperationID
	if line.OperationID != "" {
		parsed, opErr := domain.ParseOperationID(line.OperationID)
		if opErr != nil {
			return record, fmt.Errorf("operation_id: %w", opErr)
		}
		operationID = parsed
	}
	messageID, msgErr := domain.ParseMessageID(line.MessageID)
	if msgErr != nil {
		return record, fmt.Errorf("message_id: %w", msgErr)
	}
	fields, fieldErr := decodeScalarMap(line.Fields, port.LogFieldsMax)
	if fieldErr != nil {
		return record, fieldErr
	}
	decoded := port.LogRecord{
		Time: timestamp, Level: level, Invocation: invocationID,
		Operation: operationID, Component: line.Component,
		MessageID: messageID, Fields: fields,
	}
	// 書けない形のrecordを読めてしまうと、出力側の検査が空振りする。
	if err := decoded.Validate(); err != nil {
		return record, err
	}
	return decoded, nil
}

// decodeScalarMap はJSON mapをscalar parameterへ戻す（§7）。
func decodeScalarMap(source map[string]any, max int) (domain.Parameters, error) {
	if len(source) > max {
		return nil, fmt.Errorf("fieldsが%d件を超える（%d件）", max, len(source))
	}
	if len(source) == 0 {
		return nil, nil
	}
	parameters := make(domain.Parameters, len(source))
	for key, raw := range source {
		if err := domain.ValidateParameterKey(key); err != nil {
			return nil, fmt.Errorf("fields: %w", err)
		}
		scalar, err := jsonToScalar(raw)
		if err != nil {
			return nil, fmt.Errorf("fields[%q]: %w", key, err)
		}
		parameters[key] = scalar
	}
	return parameters, nil
}

// jsonToScalar はJSON値またはencoder出力をscalarへ戻す（§7）。
//
// `json.Number`はdecodeJSONのUseNumberが返す形、`int64`は[encodeScalarMap]が
// 返す形である。encode経路もdecodeと同じ検査を通すため（[EncodeEnvelope]・
// [EncodePlan]）、両方を受け取れなければ自分が書いた値を読み直せない。
func jsonToScalar(raw any) (domain.Scalar, error) {
	switch value := raw.(type) {
	case nil:
		return domain.NullScalar(), nil
	case string:
		return domain.StringScalar(value), nil
	case bool:
		return domain.BoolScalar(value), nil
	case int64:
		if _, err := requireNonNegativeOrNegativeSafe(value); err != nil {
			return domain.Scalar{}, err
		}
		return domain.IntScalar(value), nil
	case json.Number:
		// decodeJSONがUseNumberを設定しているため、数値はjson.Numberで届く。
		// float64経由にすると大きなintegerが黙って丸まる。
		integer, err := value.Int64()
		if err != nil {
			return domain.Scalar{}, fmt.Errorf("integerでない数値 %q", value.String())
		}
		if _, err := requireNonNegativeOrNegativeSafe(integer); err != nil {
			return domain.Scalar{}, err
		}
		return domain.IntScalar(integer), nil
	default:
		return domain.Scalar{}, fmt.Errorf("scalarでない値 %T", raw)
	}
}
