// Package selection は user/project 選択、current リンク、優先順位の解決を担う
// （03章6/9節、05章6節、08章10節）。
//
// 有効選択は CLI 明示 > project > user の順で解決し、disabled は fallback を止める。
// user 選択は version/variant/install ID を state へ固定し、link 方式は current link、
// backend 方式は selector snapshot を更新する。project 選択は .gdtvm.toml を正とする。
package selection
