// Package security は SHA-256 計算、承認、path 検査、secret マスクを担う（11章）。
//
// 信頼境界は下位入力から上位 trust を作らない。path containment は lexical clean と
// realpath の双方で検査し、archive 安全性・digest 検証・承認 fingerprint を fail closed
// で扱う。log/表示前に Authorization/Cookie/token/credential をマスクする。
package security
