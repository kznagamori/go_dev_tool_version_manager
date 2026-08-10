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
	"internal/app.calVerRe":                        "docs/11-quality-and-ci.md §2のCalVer grammar。compile済みregexpで、初期化後に再代入しない",
	"internal/app.commitRe":                        "40桁小文字hexのcommit ID grammar。同上",
	"internal/app.goToolchainRe":                   "go toolchain名のgrammar。同上",
	"internal/app.buildTargets":                    "docs/11-quality-and-ci.md §3のrelease target exact 2件。初期化後に変更しない対応表",
	"internal/domain.numRe":                        "docs/06-tool-definition.md §4のversion数値要素grammar。compile済みregexpで、初期化後に再代入しない",
	"internal/domain.semverRe":                     "semver grammar。同上",
	"internal/domain.goRe":                         "go scheme grammar。同上",
	"internal/domain.pythonRe":                     "python scheme grammar。同上",
	"internal/domain.semverIdent":                  "semver prerelease識別子grammar。同上",
	"internal/domain.toolIDRe":                     "tool IDのkebab-case grammar。同上",
	"internal/domain.lowerHexRe":                   "digest hexのgrammar。同上",
	"internal/domain.hexLength":                    "docs/04-storage-and-data.md §6のalgorithm別hex長。初期化後に変更しない対応表",
	"internal/domain.platforms":                    "docs/02-architecture.md §3の固定platform表。同上",
	"internal/domain.pathRoles":                    "docs/04-storage-and-data.md §17.2の22 role表。同上",
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
