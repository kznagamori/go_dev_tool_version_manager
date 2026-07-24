package app

import (
	"fmt"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/events"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
)

// Dependencies は Application Service 生成時に明示注入する依存の集合である（10章2節）。
// すべて抽象 port と解決済み設定値だけで構成し、具体的な OS/HTTP/TOML/CLI 型を含めない。
// package global mutable state を排し、各 Service は注入された依存だけを用いる。
type Dependencies struct {
	// Mode は解決済みの保存領域方式。
	Mode domain.Mode

	FS      ports.FileSystem
	Links   ports.LinkManager
	HTTP    ports.HTTPClient
	Process ports.ProcessRunner
	Archive ports.ArchiveExtractor
	Hash    ports.HashCalculator
	Locks   ports.LockManager

	Clock  ports.Clock
	IDs    ports.IDGenerator
	Logger ports.Logger
	Locale ports.LocaleProvider

	ReleaseVerifier   ports.ReleaseIntegrityVerifier
	SignatureVerifier ports.SignatureVerifier

	Events    events.EventSink
	Approvals events.ApprovalProvider
}

// Service は 10章の各 operation を提供する Application Service である。生成後の依存は
// 変更せず、複数 goroutine から呼出し可能とする（10章11節）。
type Service struct {
	deps Dependencies
}

// NewService は依存を検証して Service を生成する。mode が不正、または必須 port が未注入の
// 場合はエラーを返す。これは配線ミスを起動時に検出するための検査である。
func NewService(deps Dependencies) (*Service, error) {
	if _, err := domain.NewMode(string(deps.Mode)); err != nil {
		return nil, fmt.Errorf("app: 不正な mode: %w", err)
	}
	var missing []string
	check := func(name string, isNil bool) {
		if isNil {
			missing = append(missing, name)
		}
	}
	check("FileSystem", deps.FS == nil)
	check("LinkManager", deps.Links == nil)
	check("HTTPClient", deps.HTTP == nil)
	check("ProcessRunner", deps.Process == nil)
	check("ArchiveExtractor", deps.Archive == nil)
	check("HashCalculator", deps.Hash == nil)
	check("LockManager", deps.Locks == nil)
	check("Clock", deps.Clock == nil)
	check("IDGenerator", deps.IDs == nil)
	check("Logger", deps.Logger == nil)
	check("LocaleProvider", deps.Locale == nil)
	check("ReleaseIntegrityVerifier", deps.ReleaseVerifier == nil)
	check("SignatureVerifier", deps.SignatureVerifier == nil)
	check("EventSink", deps.Events == nil)
	check("ApprovalProvider", deps.Approvals == nil)
	if len(missing) > 0 {
		return nil, fmt.Errorf("app: 依存が未注入: %s", strings.Join(missing, ", "))
	}
	return &Service{deps: deps}, nil
}

// Mode は解決済みの保存領域方式を返す。
func (s *Service) Mode() domain.Mode { return s.deps.Mode }

// operationID は 1 つの operation に付与する ID を生成する（10章2節）。注入された
// IDGenerator を用いるため、Service ごとに独立し package global state を持たない。
func (s *Service) operationID() string { return s.deps.IDs.NewID() }
