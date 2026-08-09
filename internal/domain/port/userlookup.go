package port

// UserIdentity は実行中の実userである。
type UserIdentity struct {
	// Name はOS account名である。
	Name string
	// ID はLinuxのUID文字列、WindowsのSIDである。
	ID string
	// Home はOSが持つaccount homeである。
	//
	// Linuxでは実UIDのpasswd entry、WindowsではKnown Folderから得る。
	// 環境変数HOMEやXDG_*をroot決定に使わない。環境変数は利用者が上書きでき、
	// user modeのdata rootが意図しない位置になるためである（docs/09-platform.md）。
	Home string
}

// UserLookup は実user、home、owner識別を抽象化する
// （docs/02-architecture.md §4.1）。
type UserLookup interface {
	// Current は実行中の実userを返す。
	Current() (UserIdentity, error)

	// OwnerOf はpathの所有者IDを返す。
	//
	// bootstrapが引き継ぐ設定fileの所有者検査に使う。他userが置いたfileを
	// そのまま引き継がないための境界である。
	OwnerOf(path string) (string, error)
}
