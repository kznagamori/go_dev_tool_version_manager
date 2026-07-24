package ports

import (
	"context"
	"time"
)

// StdinPolicy は子プロセスの標準入力の扱いである。
type StdinPolicy string

// StdinPolicy の値。
const (
	// StdinClosed は stdin を閉じる（対話 installer を禁止する既定）。
	StdinClosed StdinPolicy = "closed"
	// StdinInherit は親の stdin を継承する（exec 等）。
	StdinInherit StdinPolicy = "inherit"
)

// ProcessSpec は外部プロセス起動の仕様である。shell を介さず executable と args を argv
// として渡す（11章7節）。executable は検証済み絶対 path であることを呼び出し側が保証する。
type ProcessSpec struct {
	Executable string
	Args       []string
	Dir        string
	// Env は子プロセスへ渡す完全な環境 map（親環境の継承は呼び出し側が構成する）。
	Env   map[string]string
	Stdin StdinPolicy
	// InheritStdio が true のとき stdio を親へ継承し、Result に出力を溜めない（exec 用）。
	InheritStdio bool
	Timeout      time.Duration
	// MaxOutputBytes は stdout/stderr capture の上限（InheritStdio=false のとき）。
	MaxOutputBytes int64
}

// ProcessResult は外部プロセスの結果である。InheritStdio 時は Stdout/Stderr を含めない。
type ProcessResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	// TimedOut は timeout により process tree を終了したことを表す。
	TimedOut bool
	Duration time.Duration
}

// ProcessRunner は argv 実行、環境、cwd、stdio、signal、exit code、timeout を扱う抽象
// port である（02章5節, 08章9節）。context cancel と timeout で process tree を終了する。
type ProcessRunner interface {
	// Run はプロセスを起動して結果を返す。context の cancel/deadline に応答する。
	Run(ctx context.Context, spec ProcessSpec) (*ProcessResult, error)
}
