package domain

import "fmt"

// PathRole はpathのlogical roleである（docs/04-storage-and-data.md §17.2）。
//
// roleを付ける目的は2つある。`doctor --report`がhome配下のpathをrole単位で確実に
// 置換できること、CIの書込み範囲検査が書込み先の封じ込めをroleで判定できること
// である。role未定義のpathを公開境界へ出さない。
type PathRole string

// PathRole のexactly 22値。§17.2で閉じており、追加も削除もこの表と同じ変更で行う。
const (
	RoleDataRoot         PathRole = "data-root"
	RoleDistributionRoot PathRole = "distribution-root"
	RoleRegistry         PathRole = "registry"
	RoleToolDefinition   PathRole = "tool-definition"
	RolePayload          PathRole = "payload"
	RoleVersionData      PathRole = "version-data"
	RoleSharedStorage    PathRole = "shared-storage"
	RoleReceipt          PathRole = "receipt"
	RoleReceiptIndex     PathRole = "receipt-index"
	RoleCatalog          PathRole = "catalog"
	RoleState            PathRole = "state"
	RoleStateBackup      PathRole = "state-backup"
	RoleShim             PathRole = "shim"
	RoleShimIndex        PathRole = "shim-index"
	RoleCurrentLink      PathRole = "current-link"
	RoleProjectFile      PathRole = "project-file"
	RoleConfig           PathRole = "config"
	RoleDownloadCache    PathRole = "download-cache"
	RoleStaging          PathRole = "staging"
	RoleTrash            PathRole = "trash"
	RoleLog              PathRole = "log"
	RoleReport           PathRole = "report"
)

// pathRoles は§17.2の22値である。件数の取り違えを検出できるよう表で持つ。
var pathRoles = map[PathRole]struct{}{
	RoleDataRoot: {}, RoleDistributionRoot: {}, RoleRegistry: {}, RoleToolDefinition: {},
	RolePayload: {}, RoleVersionData: {}, RoleSharedStorage: {}, RoleReceipt: {},
	RoleReceiptIndex: {}, RoleCatalog: {}, RoleState: {}, RoleStateBackup: {},
	RoleShim: {}, RoleShimIndex: {}, RoleCurrentLink: {}, RoleProjectFile: {},
	RoleConfig: {}, RoleDownloadCache: {}, RoleStaging: {}, RoleTrash: {},
	RoleLog: {}, RoleReport: {},
}

// PathRoleCount は§17.2が定める役割数である。
const PathRoleCount = 22

// ParsePathRole は文字列をPathRoleへ変換する。
func ParsePathRole(text string) (PathRole, error) {
	role := PathRole(text)
	if _, ok := pathRoles[role]; !ok {
		return "", fmt.Errorf("domain: path_role %q は§17.2の22値に含まれない", text)
	}
	return role, nil
}

// PathValue はroleと一体になったpathである（docs/04-storage-and-data.md §17.2）。
//
// exact keyは`role`と`path`だけである。PlanとCLI JSONの`path`はOS nativeの正規
// absolute pathとするが、typed errorは秘密値や個人pathを露出させないために
// `path`を空にしてroleだけを伝えられる。Windows user PATHのregistry valueは
// 唯一の例外で、role=`config`のまま`path`にexact locatorを入れる。
type PathValue struct {
	role PathRole
	path string
}

// NewPathValue はroleとpathの組を作る。pathの空は許す。
//
// pathの絶対性や正規性はここで検査しない。OS nativeなpath規則はplatform層の
// 責務であり、domainがOS依存の判定を持つと§1の依存方向に反するためである。
func NewPathValue(role PathRole, path string) (PathValue, error) {
	if _, ok := pathRoles[role]; !ok {
		return PathValue{}, fmt.Errorf("domain: path_role %q は§17.2の22値に含まれない", role)
	}
	return PathValue{role: role, path: path}, nil
}

// Role はlogical roleを返す。
func (p PathValue) Role() PathRole { return p.role }

// Path はpath文字列を返す。typed errorでは空になりうる。
func (p PathValue) Path() string { return p.path }

// WithoutPath はroleだけを残したPathValueを返す。
//
// typed errorやreportへ出す際に、個人pathを落としてroleだけを伝えるために使う
// （docs/04-storage-and-data.md §17.2、docs/10-security.md §9）。
func (p PathValue) WithoutPath() PathValue {
	return PathValue{role: p.role}
}

// IsZero はNewPathValueを通していない値かどうかを返す。
func (p PathValue) IsZero() bool { return p.role == "" }
