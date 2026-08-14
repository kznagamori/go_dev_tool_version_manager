// Package store は永続fileとCLI出力の厳密なserialize/deserializeを担う。
//
// docs/02-architecture.md §2が本領域へ「state、catalog、receipt、atomic write、
// structured logの出力とrotation」を割り当てている。本packageはそのうちcodec、
// すなわちbytesとtyped valueの相互変換だけを持つ。
//
// docs/04-storage-and-data.md §7がすべての永続表現へ共通の制約を課す。
//
//   - schema revisionはすべて1
//   - TOMLはUTF-8 BOMなしTOML 1.0、JSONはUTF-8 BOMなしRFC 8259
//   - 永続fileは末尾LFちょうど1つ
//   - unknown/duplicate key、型違い、enum外、上限超過、trailing dataを拒否する
//
// 「拒否する」を各fileが個別に実装すると抜けが出るため、strict decodeと§7の
// scalar検査は[codec.go]の共通層に置き、各file形式はその上でkey集合と
// 意味制約だけを表す。
//
// 依存はdomainだけで、filesystemもnetworkも触らない。bytesを受け取り
// bytesを返す純関数として保つのは、[docs/11-quality-and-ci.md]§7.2の
// 書込み範囲検査が「書込みはport経由だけ」を前提にするためである。
// 実際の読書きは呼出し側がport.FileSystem経由で行う。
package store
