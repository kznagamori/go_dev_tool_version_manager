// Package platform は Windows/Linux 固有のリンク、プロセス、権限、パスを抽象 port の
// 具体実装として提供する（09章）。
//
// Windows は directory junction、Linux は相対 symlink を用い、tool 本体を切替のために
// copy しない。libc 判定、system component の既知 path 解決、link capability probe を
// 含む。OS 差はこの層に閉じ込め、上位層は port 経由で利用する。
package platform
