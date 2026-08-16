package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// allowedGlobals はproduction pathで許すpackage-level varである。
//
// keyは`<module rootからの相対package dir>.<識別子>`、値は許可の根拠とする。
//
// docs/02-architecture.md §4は「package global mutable stateを使わない」、§12は
// 「package global singletonを置かない」と定める。ところがGoには、map、slice、
// pointerを「初期化後は変更しない」と型で宣言する手段が無い。そこで、変更しない
// ことをこの表で宣言し、実際に変更が加わればmutation検査（本fileの後半）が落ちる、
// という二段構えで守る。表に無い宣言は失敗する（fail closed）。
//
// `var _ T = (*Impl)(nil)`のようなblank識別子のcompile時assertionは記憶域を持たず
// 参照もできないため、表に載せずに通す。`_test.go`は対象外である。testが自分の
// fixtureをpackage scopeへ置くことまで禁じる理由はない。
var allowedGlobals = map[string]string{
	"internal/definition.kebabIDRe":                "docs/06-tool-definition.md §3のtool ID/aliasとscoped IDのgrammar。compile済み正規表現で、初期化後に変更しない",
	"internal/definition.commandNameRe":            "§3のcommand grammar。同上",
	"internal/definition.metadataKeyRe":            "§3のmetadata key grammar。同上",
	"internal/definition.spdxIDStringRe":           "SPDX 2.3のidstring grammar。同上",
	"internal/definition.utf8BOM":                  "§1が拒否するUTF-8 BOMのbyte列。比較にだけ使い、初期化後に変更しない",
	"internal/definition.lowerHexRe":               "§6.5のalgorithm prefixなしlowercase hex。compile済み正規表現で、初期化後に変更しない",
	"internal/definition.decimalIDRe":              "§6.6の非負decimal string（leading zeroなし）。同上",
	"internal/definition.digestHexLength":          "§6.5のdigest algorithmとhex長の対応表。sha256=64/sha512=128の閉じた集合で、初期化後に変更しない",
	"internal/definition.assetFieldOrder":          "§6.5のasset field exact 13値を仕様の並び順で持つ。順序も診断とstatic assetの必須検査の契約のためsliceで固定し、初期化後に変更しない",
	"internal/definition.sourceKeyOrder":           "§6.1のversion source許可key exact 24値を仕様の並び順で持つ。同上",
	"internal/definition.hostnameRe":               "§7.1の`redirect_hosts`が取るASCII lowercase完全host。compile済み正規表現で、初期化後に変更しない",
	"internal/definition.templateRe":               "§12のtemplate変数の出現を切り出す正規表現。同上",
	"internal/definition.storageTemplateRe":        "§12の`{{storage.<id>}}`のID抽出。同上",
	"internal/definition.metadataTemplateRe":       "§12の`{{metadata.<key>}}`のkey抽出。同上",
	"internal/definition.assetTemplateRe":          "§12の`{{asset.<field>}}`のfield抽出。同上",
	"internal/definition.artifactScope":            "§7.1のURL/file templateが許すroot集合。初期化後に変更しない",
	"internal/definition.commandScope":             "§10のcommand/environmentが許すroot集合。同上",
	"internal/definition.probeArgScope":            "§11のprobe args/required_pathsが許すroot集合。同上",
	"internal/definition.expectedVersionScope":     "§11の`expected_version`が許すroot集合。同上",
	"internal/definition.payloadOnlyScope":         "§10.1のcommand targetが許すroot集合。同上",
	"internal/domain/port.lockClassNames":          "docs/02-architecture.md §12のlock分類 exact 6値とrole名の対応表。初期化後に変更しない",
	"internal/domain/port.ErrLockOrder":            "lock順序違反を表すsentinel error。errors.Isで判定するため値として公開する。初期化後に再代入しない",
	"internal/domain/port.ErrLockTimeout":          "lock取得timeoutを表すsentinel error。同上",
	"internal/store.resultWarningCodes":            "docs/04-storage-and-data.md §16.2のResultWarningCode exact 5値。初期化後に変更しない閉じた集合",
	"internal/store.jsonCommands":                  "§17の`--json`対応command exact 5値。同上",
	"internal/store.severities":                    "§17.1のseverity exact 3値。同上",
	"internal/store.doctorStatuses":                "§17.1のdoctor_status exact 3値。同上",
	"internal/store.diagnosticCodeOrder":           "§17.1のdiagnostic_code exact 10値をcode順で持つ。順序も契約のためsliceで固定し、初期化後に変更しない",
	"internal/store.planWarningApproval":           "§16.1のPlanWarningCode 8件と承認要否の対応表。初期化後に変更しない",
	"internal/store.planOperations":                "§17.1のPlan operation exact 5値。同上",
	"internal/store.planProviderKinds":             "§17.1のprovider_kind。Planだけ`none`を含む閉じた集合",
	"internal/store.filesystemCapabilities":        "§17.1のsetup filesystem capability exact 7値。同上",
	"internal/store.linkStrategies":                "§17.1のcurrent link strategy exact 2値。同上",
	"internal/store.shimStrategies":                "§17.1のshim strategy exact 3値。同上",
	"internal/store.writeActions":                  "§17.1のwrites[].action exact 6値。同上",
	"internal/store.storageActions":                "§17.1のstorage[].action exact 3値。同上",
	"internal/store.archiveFormats":                "§17.1のarchive format exact 2値。同上",
	"internal/store.planArgKinds":                  "§17.1のPlanArg kind exact 2値。同上",
	"internal/store.requiredPathKinds":             "§17.1のprobe path kind exact 2値。同上",
	"internal/store.downloadDestinationRoles":      "§16のdownloads[].destination.roleが取りうる2値。同上",
	"internal/store.pathAbsolute":                  "§17.2のpath制約の組合せ。bool 2個のみを持つ読取り専用の設定値で、初期化後に変更しない",
	"internal/store.pathOptional":                  "同上",
	"internal/store.pathLocatorOrAbsolute":         "同上",
	"internal/store.pathOptionalLocator":           "同上",
	"internal/store.receiptProviderKinds":          "docs/04-storage-and-data.md §17.1のprovider_kind。receiptとcatalogは対象toolを必ず持つため`none`を含めない閉じた集合",
	"internal/store.checksumSources":               "§17.1のchecksum_source exact 2値。初期化後に変更しない閉じた集合",
	"internal/store.storageKinds":                  "docs/06-tool-definition.md §8のstorage kind exact 6値。同上",
	"internal/store.storageScopes":                 "§17.1のstorage scope exact 2値。同上",
	"internal/store.storagePurges":                 "§17.1のstorage purge exact 3値。同上",
	"internal/store.workingDirectories":            "§17.1のworking_directory exact 2値。同上",
	"internal/store.probeStreams":                  "§17.1のprobe stream exact 3値。同上",
	"internal/store.probeExpects":                  "§17.1のprobe expect exact 3値。同上",
	"internal/store.probeStatuses":                 "§17.1のprobe status exact 2値。同上",
	"internal/store.requiredPathPrefixes":          "docs/06-tool-definition.md §11のrequired path prefix exact 2値。同上",
	"internal/store.clientCommitRe":                "40桁小文字hexのcommit ID grammar。compile済みregexpで、初期化後に再代入しない",
	"internal/store.identifierRe":                  "docs/06-tool-definition.md §3のkebab-case identifier grammar。同上",
	"internal/store.envNameRe":                     "環境変数名のgrammar。同上",
	"internal/store.storageTemplateRe":             "`{{storage.<id>}}` templateのgrammar。同上",
	"internal/store.templateRootRe":                "未知`{{...}}`検出用のgrammar。同上",
	"internal/store.commandScope":                  "docs/04-storage-and-data.md §14のcommand/storage/env用template許可root。初期化後に変更しない設定値",
	"internal/store.probeScope":                    "docs/06-tool-definition.md §11のprobe用template許可root。同上",
	"internal/store.utf8BOM":                       "docs/04-storage-and-data.md §7が拒否するUTF-8 BOMのbyte列。読取り専用の定数相当で、初期化後に変更しない",
	"internal/store.relativePathRe":                "§7のPOSIX relative path grammar。compile済みregexpで、初期化後に再代入しない",
	"internal/store.idHexRe":                       "§7の128 bit ID（32 lowercase hex）grammar。同上",
	"internal/store.clientVersionRe":               "docs/11-quality-and-ci.md §2のCalVer grammar。同上",
	"internal/store.commandNameRe":                 "§12のshim command名grammar。同上",
	"internal/store.pathIntegrations":              "§17.1のpath_integration exact 3値。初期化後に変更しない閉じた集合",
	"internal/store.shells":                        "§17.1のshell exact 3値。同上",
	"internal/store.integrationKinds":              "§17.1のintegration_identity.kind exact 3値。同上",
	"internal/store.backupKinds":                   "§17.1のsetup backup kind exact 2値。同上",
	"internal/app.calVerRe":                        "docs/11-quality-and-ci.md §2のCalVer grammar。compile済みregexpで、初期化後に再代入しない",
	"internal/app.commitRe":                        "40桁小文字hexのcommit ID grammar。同上",
	"internal/app.goToolchainRe":                   "go toolchain名のgrammar。同上",
	"internal/app.buildTargets":                    "docs/11-quality-and-ci.md §3のrelease target exact 2件。初期化後に変更しない対応表",
	"internal/domain.numRe":                        "docs/06-tool-definition.md §4のversion数値要素grammar。compile済みregexpで、初期化後に再代入しない",
	"internal/domain.semverRe":                     "semver grammar。同上",
	"internal/domain.goRe":                         "go scheme grammar。同上",
	"internal/domain.pythonRe":                     "python scheme grammar。同上",
	"internal/domain.semverIdentRe":                "semver prerelease識別子grammar。同上",
	"internal/domain.toolIDRe":                     "tool IDのkebab-case grammar。同上",
	"internal/domain.lowerHexRe":                   "digest hexのgrammar。同上",
	"internal/domain.hexLength":                    "docs/04-storage-and-data.md §6のalgorithm別hex長。初期化後に変更しない対応表",
	"internal/domain.platforms":                    "docs/02-architecture.md §3の固定platform表。同上",
	"internal/domain.pathRoles":                    "docs/04-storage-and-data.md §17.2の22 role表。同上",
	"internal/config.colorModes":                   "docs/05-configuration.md §3.1のcolor 3値。初期化後に変更しない対応表",
	"internal/domain.exitCodes":                    "docs/03-cli.md §7のerror code→終了code写像。初期化後に変更しない対応表",
	"internal/domain.nonRetryableCodes":            "docs/02-architecture.md §14がretryable=trueを禁じる8件。同上",
	"internal/domain.idRe":                         "128 bit IDの32桁小文字hex grammar。compile済みregexpで、初期化後に再代入しない",
	"internal/domain.messageIDRe":                  "docs/04-storage-and-data.md §7のmessage ID grammar。同上",
	"internal/domain.parameterKeyRe":               "同§7のscalar parameter key grammar。同上",
	"internal/domain/port.logLevels":               "docs/04-storage-and-data.md §18のlog level 5値。初期化後に変更しない対応表",
	"internal/domain/port/fake.levelOrder":         "fake Loggerのlevel詳細度表。同上",
	"internal/progress.phases":                     "docs/02-architecture.md §10のphase 10値。同上",
	"internal/progress.units":                      "同§10のunit 4値。同上",
	"internal/progress.resultWarningCodes":         "docs/04-storage-and-data.md §16.2のResultWarningCode 5値。同上",
	"internal/security.windowsReservedNames":       "docs/04-storage-and-data.md §6のWindows予約device名。初期化後に変更しない対応表",
	"internal/security.secretEnvSuffixes":          "docs/10-security.md §9.2の除去対象環境変数名pattern。初期化後に変更しないslice",
	"internal/security.secretHeaders":              "同§9.2の除去対象HTTP header。初期化後に変更しない対応表",
	"internal/domain/port/fake.ErrNotExist":        "sentinel error。errors.Isの比較対象であり、初期化後に再代入しない",
	"internal/domain/port/fake.ErrDiskFull":        "sentinel error。同上",
	"internal/domain/port/fake.ErrDownloadFailed":  "sentinel error。同上",
	"internal/domain/port/fake.ErrProbeFailed":     "sentinel error。同上",
	"internal/domain/port/fake.ErrLinkUnsupported": "sentinel error。同上",
}

// TestNoPackageGlobalMutableState はproduction pathのpackage-level varを検査する。
//
// 検査は2段である。
//
//  1. 宣言検査: blank識別子以外のpackage-level varが[allowedGlobals]にあること。
//  2. 変更検査: 許可した識別子が、宣言以降どこでも代入・increment・address取得の
//     対象になっていないこと。表へ載せた「初期化後に変更しない」という宣言が、
//     後から静かに破られるのを防ぐ。
//
// module全体をsource levelで見るため、`internal/app`のtestでありながら他package
// も対象にする。docs/02-architecture.md §4がこの不変条件をconstructorの規定と
// 同じ場所で定めており、その規定を実装するのが本packageの[NewServices]である。
func TestNoPackageGlobalMutableState(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	byPackage := parseProductionPackages(t, fset, root)

	declared := make(map[string]map[string]bool) // pkgDir -> 識別子集合
	for _, pkgDir := range sortedKeys(byPackage) {
		names := make(map[string]bool)
		for _, file := range byPackage[pkgDir] {
			for _, name := range packageLevelVars(file) {
				if name.Name == "_" {
					continue
				}
				key := pkgDir + "." + name.Name
				if _, ok := allowedGlobals[key]; !ok {
					t.Errorf("%s: package-level var %q が allowedGlobals に無い。"+
						"docs/02-architecture.md §4 はpackage global mutable stateを禁じる。"+
						"instance fieldへ移すか、変更しない根拠を添えて表へ登録する",
						position(fset, root, name.Pos()), key)
					continue
				}
				names[name.Name] = true
			}
		}
		declared[pkgDir] = names
	}

	for _, pkgDir := range sortedKeys(byPackage) {
		names := declared[pkgDir]
		if len(names) == 0 {
			continue
		}
		for _, file := range byPackage[pkgDir] {
			for _, m := range mutations(file, names) {
				t.Errorf("%s: package-level var %q を変更している。"+
					"allowedGlobals は「初期化後に変更しない」ことを条件に許可している",
					position(fset, root, m.pos), pkgDir+"."+m.name)
			}
		}
	}
}

// TestAllowedGlobalsHasNoStaleEntry は表に残った未使用entryを検出する。
//
// 許可entryを消し忘れると、次に同じ名前のvarを作ったときに無審査で通ってしまう。
func TestAllowedGlobalsHasNoStaleEntry(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	byPackage := parseProductionPackages(t, fset, root)

	found := make(map[string]bool)
	for pkgDir, files := range byPackage {
		for _, file := range files {
			for _, name := range packageLevelVars(file) {
				found[pkgDir+"."+name.Name] = true
			}
		}
	}
	for _, key := range sortedKeys(allowedGlobals) {
		if !found[key] {
			t.Errorf("allowedGlobals の %q は存在しない宣言である。表から削除する", key)
		}
	}
}

// TestAllowedGlobalsHasReason は根拠の無いentryを拒否する。
func TestAllowedGlobalsHasReason(t *testing.T) {
	for _, key := range sortedKeys(allowedGlobals) {
		if strings.TrimSpace(allowedGlobals[key]) == "" {
			t.Errorf("allowedGlobals の %q に許可の根拠が無い", key)
		}
	}
}

// mutationSite は許可済みglobalへの変更箇所である。
type mutationSite struct {
	name string
	pos  token.Pos
}

// mutations はfile内で names の識別子を変更している箇所を集める。
//
// 代入（`x = v`、`x[k] = v`、`*x = v`、`x.f = v`）、increment/decrement、
// address取得（`&x`）を対象にする。address取得を含めるのは、pointerを渡した先で
// 変更されると代入としては現れないためである。
//
// 同名のlocal変数への代入も違反として報告する。scope解決を持ち込まずに
// fail closedへ倒しており、衝突したlocalは改名すれば解消できる。
func mutations(file *ast.File, names map[string]bool) []mutationSite {
	var sites []mutationSite
	record := func(expr ast.Expr) {
		if ident := rootIdent(expr); ident != nil && names[ident.Name] {
			sites = append(sites, mutationSite{name: ident.Name, pos: ident.Pos()})
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				record(lhs)
			}
		case *ast.IncDecStmt:
			record(n.X)
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				record(n.X)
			}
		}
		return true
	})
	return sites
}

// rootIdent は式の一番外側にある被参照識別子を返す。
//
// `x[k].f` のような式から `x` を取り出す。識別子へ辿り着かない式ではnilを返す。
func rootIdent(expr ast.Expr) *ast.Ident {
	for {
		switch e := expr.(type) {
		case *ast.Ident:
			return e
		case *ast.IndexExpr:
			expr = e.X
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.StarExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.SliceExpr:
			expr = e.X
		default:
			return nil
		}
	}
}

// packageLevelVars はfile scopeの`var`宣言が導入する識別子を返す。
func packageLevelVars(file *ast.File) []*ast.Ident {
	var names []*ast.Ident
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			names = append(names, value.Names...)
		}
	}
	return names
}

// parseProductionPackages はmodule内のproduction Go fileをpackage dirごとに解析する。
//
// `_test.go`、`testdata`配下、dot始まりのdirectoryは除く。
func parseProductionPackages(t *testing.T, fset *token.FileSet, root string) map[string][]*ast.File {
	t.Helper()

	byPackage := make(map[string][]*ast.File)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		pkgDir := filepath.ToSlash(mustRel(t, root, filepath.Dir(path)))
		byPackage[pkgDir] = append(byPackage[pkgDir], file)
		return nil
	})
	if err != nil {
		t.Fatalf("production Go fileの走査に失敗した: %v", err)
	}
	if len(byPackage) == 0 {
		t.Fatal("production Go fileが1件も見つからない。走査範囲が誤っている")
	}
	return byPackage
}

// moduleRoot は`go.mod`を持つ祖先directoryを返す。
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("current directoryを取得できない: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.modを持つmodule rootが見つからない")
		}
		dir = parent
	}
}

func mustRel(t *testing.T, base, target string) string {
	t.Helper()

	rel, err := filepath.Rel(base, target)
	if err != nil {
		t.Fatalf("relative pathを求められない: %v", err)
	}
	return rel
}

// position は個人pathを出さないよう、module root相対の位置文字列を返す。
func position(fset *token.FileSet, root string, pos token.Pos) string {
	p := fset.Position(pos)
	if rel, err := filepath.Rel(root, p.Filename); err == nil {
		p.Filename = filepath.ToSlash(rel)
	}
	return p.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
