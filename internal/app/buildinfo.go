package app

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BuildInfo はrelease binaryへ埋め込むbuild metadataである。
//
// 内容はdocs/11-quality-and-ci.md §4が定める。値はbuild時にlink flagから決まり、
// runtimeにVERSION fileやnetworkから読み直さない（同§4）。そのため型としては
// 単なる搬送用structとし、正しさの判定は[BuildInfo.Validate]に集約する。
//
// docs/02-architecture.md §4が「constructorは依存の存在とbuild metadata形式だけを
// 検査する」と定めるため、検査の呼出し点は[NewServices]である。値渡しで扱い、
// [Services]は自分のcopyを持つので、生成後に呼出し側が書き換えても影響しない。
type BuildInfo struct {
	// ClientVersion はdocs/11-quality-and-ci.md §2のCalVer `YYYY.MM.DD.XX`である。
	// development buildだけ`devel`を持てる。
	ClientVersion string
	// ClientRelease はrelease binaryかどうかを表す。
	ClientRelease bool
	// Commit は40桁小文字hexのcommit IDである。
	Commit string
	// Dirty はbuild時の作業treeに未commit差分があったかを表す。
	// release binaryでは常にfalseでなければならない（§4）。
	Dirty bool
	// BuiltAt はUTCのbuild時刻である。
	BuiltAt time.Time
	// GoToolchain はbuildに使ったtoolchain（例 `go1.26.5`）である。
	GoToolchain string
	// GOOS/GOARCH/CGOEnabled はdocs/11-quality-and-ci.md §3のrelease targetである。
	GOOS       string
	GOARCH     string
	CGOEnabled bool
	// DefinitionSchema/RegistrySchema/StateSchema はこのclientが対応する
	// 各schema versionである（§4）。
	DefinitionSchema int
	RegistrySchema   int
	StateSchema      int
	// RepositoryOwner/RepositoryName は公式GitHub repositoryの識別子である（§4）。
	RepositoryOwner string
	RepositoryName  string
}

// devVersion はdevelopment buildだけが名乗れるclient versionである（§2）。
const devVersion = "devel"

// calVerRe はdocs/11-quality-and-ci.md §2が明示したgrammarである。
// 実在日付の判定は別途行う（`2026.02.30.00`は文字種としては通るため）。
var calVerRe = regexp.MustCompile(`^[0-9]{4}[.](0[1-9]|1[0-2])[.](0[1-9]|[12][0-9]|3[01])[.][0-9]{2}$`)

// commitRe はgit object IDの40桁小文字hexである（§2）。
var commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// goToolchainRe はGoのtoolchain名（`goMAJOR.MINOR[.PATCH]`）である。
// 具体的にどのversionを要求するかはdocs/11-quality-and-ci.md §1.2の`lint` jobが
// `go.mod`と照合する。ここは形式だけを見る。
var goToolchainRe = regexp.MustCompile(`^go[1-9][0-9]*[.](0|[1-9][0-9]*)([.](0|[1-9][0-9]*))?$`)

// buildTargets はdocs/11-quality-and-ci.md §3が定めるrelease targetのexact 2件である。
var buildTargets = map[string]struct{}{
	"windows/amd64": {},
	"linux/amd64":   {},
}

// Validate はbuild metadataの形式を検査する。
//
// filesystem、network、環境変数を読まない。docs/02-architecture.md §4が
// constructorへ許すのは「依存の存在とbuild metadata形式」の検査だけであり、
// 実際に埋め込まれた値が正しいrelease由来かどうかの照合はCI（§4末尾の
// binary/VERSION/tag/archive名の不一致検査）の責務だからである。
//
// 誤りは1件目で打ち切らず全件を返す。build metadataはlink flagの組立て誤りで
// 複数fieldが同時に欠けることが多く、1件ずつ直すと再buildを繰り返すためである。
func (b BuildInfo) Validate() error {
	var errs []error

	errs = append(errs, b.validateVersionIdentity()...)

	if !b.BuiltAt.IsZero() {
		if zone, offset := b.BuiltAt.Zone(); offset != 0 || zone != "UTC" {
			errs = append(errs, fmt.Errorf("app: build時刻はUTCで持つ（%s%+d が渡された）", zone, offset))
		}
	} else {
		errs = append(errs, errors.New("app: build時刻が未設定"))
	}

	if !goToolchainRe.MatchString(b.GoToolchain) {
		errs = append(errs, fmt.Errorf("app: go toolchain %q が `goMAJOR.MINOR[.PATCH]` 形式でない", b.GoToolchain))
	}

	target := b.GOOS + "/" + b.GOARCH
	if _, ok := buildTargets[target]; !ok {
		errs = append(errs, fmt.Errorf(
			"app: build target %q はdocs/11-quality-and-ci.md §3の windows/amd64|linux/amd64 でない", target))
	}
	if b.CGOEnabled {
		errs = append(errs, errors.New("app: §3のrelease targetはCGO=0であり、CGO有効buildを受け付けない"))
	}

	errs = append(errs, validateSchema("definition", b.DefinitionSchema))
	errs = append(errs, validateSchema("registry", b.RegistrySchema))
	errs = append(errs, validateSchema("state", b.StateSchema))

	errs = append(errs, validateRepositoryPart("owner", b.RepositoryOwner))
	errs = append(errs, validateRepositoryPart("name", b.RepositoryName))

	return errors.Join(errs...)
}

// validateVersionIdentity はversion、release flag、commit、dirtyの整合を見る。
//
// docs/11-quality-and-ci.md §2は`devel`とdirty=trueをdevelopment buildだけに許し、
// §4はrelease binaryのdirtyをfalseと定める。両者を1か所で判定する。
func (b BuildInfo) validateVersionIdentity() []error {
	var errs []error

	switch {
	case b.ClientVersion == devVersion:
		if b.ClientRelease {
			errs = append(errs, fmt.Errorf(
				"app: client version %q はdevelopment buildだけが名乗れる（client_release=trueが渡された）", devVersion))
		}
	default:
		if err := validateCalVer(b.ClientVersion); err != nil {
			errs = append(errs, err)
		}
	}

	if b.ClientRelease && b.Dirty {
		errs = append(errs, errors.New("app: release binaryはdirty=falseでなければならない"))
	}

	if !commitRe.MatchString(b.Commit) {
		errs = append(errs, errors.New("app: commitが40桁小文字hexでない"))
	}

	return errs
}

// validateCalVer はCalVer grammarと実在日付を検査する（docs/11-quality-and-ci.md §2）。
func validateCalVer(version string) error {
	if !calVerRe.MatchString(version) {
		return fmt.Errorf("app: client version %q が `YYYY.MM.DD.XX` 形式でも %q でもない", version, devVersion)
	}
	parts := strings.Split(version, ".")
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("app: client version %q の年を解釈できない", version)
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("app: client version %q の月を解釈できない", version)
	}
	day, err := strconv.Atoi(parts[2])
	if err != nil {
		return fmt.Errorf("app: client version %q の日を解釈できない", version)
	}
	// time.Dateは範囲外の日を翌月へ繰り上げるため、往復させて実在日だけを通す。
	got := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if got.Year() != year || got.Month() != time.Month(month) || got.Day() != day {
		return fmt.Errorf("app: client version %q の日付が実在しない", version)
	}
	return nil
}

// validateSchema は対応schema versionが1以上かを検査する。
//
// 上限を持たないのは、clientが将来のschemaへ対応したときにこの検査を変えずに
// 済ませるためである。具体的な値との照合はregistry読込み側の責務である。
func validateSchema(kind string, value int) error {
	if value < 1 {
		return fmt.Errorf("app: %s schema versionは1以上だが %d が渡された", kind, value)
	}
	return nil
}

// validateRepositoryPart は`owner/name`へ合成できる断片かを検査する。
//
// 空、空白、`/`を拒否するのは、2つを連結したときに別のrepositoryを指す文字列が
// できないようにするためである。GitHub側の命名規則の完全な再現はしない。
func validateRepositoryPart(kind, value string) error {
	switch {
	case value == "":
		return fmt.Errorf("app: repository %sが未設定", kind)
	case strings.ContainsAny(value, "/ \t\r\n"):
		return fmt.Errorf("app: repository %s %q に `/` または空白が含まれる", kind, value)
	}
	return nil
}
