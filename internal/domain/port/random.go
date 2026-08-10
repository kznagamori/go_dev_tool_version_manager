package port

import "github.com/kznagamori/go_dev_tool_version_manager/internal/domain"

// Random は128 bit ID生成のportである（docs/02-architecture.md §4.1）。
//
// invocation ID、operation ID、install ID、root ID、lock IDはすべてこのportから
// 得た乱数を[domain.NewInvocationID]等で正規形へ変換する。生成をportにするのは、
// testで固定IDを与えてPlan、receipt、log、lock metadataをgolden比較できるように
// するためである。
//
// 生成に失敗しうる（OSのentropy source取得失敗）ためerrorを返す。IDを推測可能な
// 値で代用するfallbackを実装しない。
type Random interface {
	// NewIDBytes は128 bitのrandom byte列を返す。
	NewIDBytes() ([domain.IDByteLength]byte, error)
}
