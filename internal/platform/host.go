// Package platform のhost platform判定である。
package platform

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// ELF identity headerの位置と値（System V ABI、ELF-64 Object File Format）。
//
// loaderが「その名前のfile」ではなく**実体としてx86-64のELFである**ことを確かめる
// ために読む。docs/09-platform.md §5.3「libcは実体loader/libc identityとprobeで
// 判定し、環境変数や`ldd`表示だけで決めない」。
const (
	// elfMagic はELF fileの先頭4 byteである。
	elfMagic = "\x7fELF"
	// elfIdentSize はe_machineまでを読むのに要るbyte数である。
	elfIdentSize = 20
	// elfClassOffset はEI_CLASSの位置である。
	elfClassOffset = 4
	// elfDataOffset はEI_DATAの位置である。
	elfDataOffset = 5
	// elfMachineOffset はe_machineの位置である（EI_NIDENT=16、e_type=2 byte）。
	elfMachineOffset = 18
	// elfClass64 はEI_CLASSのELFCLASS64である。
	elfClass64 = 2
	// elfDataLSB はEI_DATAのELFDATA2LSBである。
	elfDataLSB = 1
	// elfMachineX8664 はe_machineのEM_X86_64である。
	elfMachineX8664 = 0x3E
)

// LibcKind はloader観測が示すlibcである。
//
// **[domain.Libc]と別の型にしている。** domain.Libcは「対象platformが持つlibc」で
// あり、v0.1の表にはmuslが無い。対象外を表せる語彙をdomainへ足すと、どのPlatformも
// 取り得ない値がdomainに残る（CLAUDE.md §7）。観測の語彙はadapter側に置く。
type LibcKind string

// LibcKind の値。
const (
	// LibcKindGlibc はGNU C Libraryである。
	LibcKindGlibc LibcKind = "glibc"
	// LibcKindMusl はmusl libcである。v0.1では対象外だが、対象外と述べるために要る。
	LibcKindMusl LibcKind = "musl"
)

// libcProbe は1件のlibc判定候補である。
type libcProbe struct {
	// path は観測するloaderまたはlibraryのpathである。
	path string
	// kind はそのpathが実在した場合に示すlibcである。
	kind LibcKind
}

// linuxLibcProbes はLinuxで観測するloader/libraryである。
//
// glibcはABIがloader pathを固定しており（x86-64は`/lib64/ld-linux-x86-64.so.2`）、
// distribution差はlibc本体の置き場に出る。muslは別のloader pathを使うため、
// **両方を観測して食い違いを検出できる**ようにする。
var linuxLibcProbes = []libcProbe{
	{path: "/lib64/ld-linux-x86-64.so.2", kind: LibcKindGlibc},
	{path: "/lib/x86_64-linux-gnu/libc.so.6", kind: LibcKindGlibc},
	{path: "/lib64/libc.so.6", kind: LibcKindGlibc},
	{path: "/lib/ld-musl-x86_64.so.1", kind: LibcKindMusl},
}

// kernelOSNames はkernelが報告するOS名からdomain.OSへの表である。
//
// 実行時に合成せず表で閉じる。表に無い名前は対象外として扱う。
var kernelOSNames = map[string]domain.OS{
	"linux":   domain.OSLinux,
	"windows": domain.OSWindows,
}

// kernelArchNames はkernelが報告するmachine名からdomain.Archへの表である。
//
// v0.1が対象とするのはamd64だけである。**対象外archを表へ載せない** ——
// 載せるとdomain.Archに対応する値が無く、取り得ない値を作ることになる。
var kernelArchNames = map[string]domain.Arch{
	"x86_64": domain.ArchAMD64,
	"amd64":  domain.ArchAMD64,
}

// LibcEvidence はlibc判定の根拠1件である。
//
// `doctor`が「なぜこのhostを対象外と判定したか」を利用者へ示せるよう、判定結果
// だけでなく観測そのものを残す（CLAUDE.md §12）。
type LibcEvidence struct {
	// Path は観測したloaderまたはlibraryのpathである。
	Path string
	// Kind はそのpathが示すlibcである。
	Kind LibcKind
}

// HostFacts はhost判定に使う観測値である。
//
// **判定（[ClassifyHost]）と観測（[DetectHost]）を分ける。** 観測はsyscallと
// filesystemを読むためtestで再現できないが、判定は純粋な変換であり、musl・arm64・
// 未知kernelを網羅して確かめられる。
type HostFacts struct {
	// KernelOS はkernelが報告するOS名である。
	//
	// docs/09-platform.md §5.3「OS/archはkernel APIで判定」。`runtime.GOOS`は
	// buildした対象であってhostではない。
	KernelOS string
	// KernelArch はkernelが報告するmachine名である。
	//
	// **`runtime.GOARCH`で代用しない。** Windows on ARMやqemuではamd64 binaryが
	// arm64 kernel上で動き、`runtime.GOARCH`はamd64を返す。対象外hostを対象と
	// 誤判定する。
	KernelArch string
	// LibcEvidences は観測できたlibcの根拠である。
	LibcEvidences []LibcEvidence
	// LongPathsEnabled はMAX_PATHを超えるpathを扱えるかである。
	//
	// docs/09-platform.md §3.1「long path利用可否をAPIで扱い、`MAX_PATH`へ暗黙
	// truncateしない」。Windowsだけが意味を持ち、Linuxには相当する切替が無いため
	// 常にtrueとする。
	LongPathsEnabled bool
}

// HostPlatform は実行中hostのplatformと判定根拠である。
type HostPlatform struct {
	// Platform は判定したplatformである。判定できなかった場合は零値である。
	Platform domain.Platform
	// Facts は判定に使った観測値である。
	Facts HostFacts
}

// ClassifyHost は観測値からhost platformを決める。
//
// docs/09-platform.md §1「arm64、Linux musl等は追加手順を経るまで非対応」、
// §5.3「musl、WSL上の異種filesystem、containerで必須能力が欠ける場合は
// `E_PLATFORM_UNSUPPORTED`」。
//
// **判定できない場合に既定値へ落とさない。** 対象外hostで動かすと、glibc向け
// artifactを導入して実行時に初めて壊れる。判定できないことをそのまま返す。
func ClassifyHost(facts HostFacts) (domain.Platform, *domain.Error) {
	hostOS, ok := kernelOSNames[normalizeKernelName(facts.KernelOS)]
	if !ok {
		return domain.Platform{}, unsupportedHost(fmt.Sprintf(
			"kernelが報告したOS %q は対象外である（対象は%s、%s）",
			facts.KernelOS, domain.OSWindows, domain.OSLinux))
	}
	hostArch, ok := kernelArchNames[normalizeKernelName(facts.KernelArch)]
	if !ok {
		return domain.Platform{}, unsupportedHost(fmt.Sprintf(
			"kernelが報告したarchitecture %q は対象外である（対象は%s）",
			facts.KernelArch, domain.ArchAMD64))
	}
	libc, libcErr := classifyLibc(hostOS, facts.LibcEvidences)
	if libcErr != nil {
		return domain.Platform{}, libcErr
	}
	return platformFor(hostOS, hostArch, libc)
}

// normalizeKernelName はkernelが返す名前を表引き用に整える。
//
// uname(2)の値は末尾空白やcaseがhostで揺れる。表の側を増やさず入力を整える。
func normalizeKernelName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// classifyLibc は観測したloaderからlibcを決める。
func classifyLibc(hostOS domain.OS, evidences []LibcEvidence) (domain.Libc, *domain.Error) {
	if hostOS == domain.OSWindows {
		// WindowsにC標準libraryの選択は無い。loaderを観測する意味がない。
		return domain.LibcNone, nil
	}
	found := make(map[LibcKind]bool, len(evidences))
	for _, evidence := range evidences {
		found[evidence.Kind] = true
	}
	switch {
	case found[LibcKindGlibc] && found[LibcKindMusl]:
		// gcompatを入れたAlpineや多重libcのcontainerで起こる。**どちらのartifactを
		// 導入すべきか決められない状態を、片方に決めて進めない。**
		return "", unsupportedHost(fmt.Sprintf(
			"glibcとmuslのloaderが同時に見つかり、libcを一意に決められない（%s）",
			describeEvidence(evidences)))
	case found[LibcKindMusl]:
		return "", unsupportedHost(fmt.Sprintf(
			"muslのloaderを検出した。v0.1が対象とするのはglibcだけである（%s）",
			describeEvidence(evidences)))
	case found[LibcKindGlibc]:
		return domain.LibcGlibc, nil
	default:
		// 見つからないことをglibcと見なさない。導入するartifactはglibc向けであり、
		// 根拠なく進めると実行時に初めて壊れる。
		return "", unsupportedHost(
			"glibcのloaderを検出できなかった。v0.1が対象とするのはglibcだけである")
	}
}

// platformFor はtupleからdomain.Platformを引く。
func platformFor(
	hostOS domain.OS, hostArch domain.Arch, libc domain.Libc,
) (domain.Platform, *domain.Error) {
	id := domain.PlatformLinuxAMD64Glibc
	if hostOS == domain.OSWindows {
		id = domain.PlatformWindowsAMD64
	}
	platform, err := domain.ParsePlatform(id)
	if err != nil {
		return domain.Platform{}, domain.Internal(err)
	}
	// 表から引いた値が観測と食い違わないことを確かめる。食い違うのは表の側の誤り
	// であり、黙って観測を捨てない。
	if platform.OS() != hostOS || platform.Arch() != hostArch || platform.Libc() != libc {
		return domain.Platform{}, domain.Internal(fmt.Errorf(
			"platform: 観測(%s/%s/%s)がplatform表の%s(%s/%s/%s)と一致しない",
			hostOS, hostArch, libc,
			platform.ID(), platform.OS(), platform.Arch(), platform.Libc()))
	}
	return platform, nil
}

// describeEvidence は観測をerror文へ載せる形にする。
func describeEvidence(evidences []LibcEvidence) string {
	if len(evidences) == 0 {
		return "観測なし"
	}
	parts := make([]string, 0, len(evidences))
	for _, evidence := range evidences {
		parts = append(parts, fmt.Sprintf("%s=%s", evidence.Path, evidence.Kind))
	}
	// 観測順はfilesystemの都合で変わりうる。error文を安定させるため並べる。
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// unsupportedHost は対象外hostのerrorを作る。
//
// docs/09-platform.md §9の`E_PLATFORM_UNSUPPORTED`である。retryしても解消しない
// ためRetryableにしない。
func unsupportedHost(reason string) *domain.Error {
	return &domain.Error{
		Code:  domain.CodePlatformUnsupported,
		Cause: fmt.Errorf("platform: %s", reason),
	}
}

// DetectHost は実行中hostのplatformを判定する。
//
// 観測はOS固有であり、判定は[ClassifyHost]が行う。判定に失敗した場合も観測値を
// 返す —— `doctor`が「何を見て対象外と判定したか」を示せるようにするためである。
func DetectHost() (HostPlatform, *domain.Error) {
	facts, err := collectHostFacts()
	if err != nil {
		return HostPlatform{}, &domain.Error{
			Code:  domain.CodePlatformUnsupported,
			Cause: fmt.Errorf("platform: host情報を取得できない: %w", err),
		}
	}
	platform, classifyErr := ClassifyHost(facts)
	if classifyErr != nil {
		return HostPlatform{Facts: facts}, classifyErr
	}
	return HostPlatform{Platform: platform, Facts: facts}, nil
}

// probeLibc は表のpathを観測してlibcの根拠を集める。
//
// 存在するだけでは採らず、**x86-64のELFとして読めることまで確かめる**。path名
// だけで決めると、名前が同じだけの別fileや壊れたlinkをloaderと見なす。
func probeLibc(probes []libcProbe) []LibcEvidence {
	evidences := make([]LibcEvidence, 0, len(probes))
	seen := make(map[LibcKind]bool, len(probes))
	for _, probe := range probes {
		if seen[probe.kind] {
			// 同じlibcの根拠を重ねても判定は変わらない。error文が長くなるだけ
			// なので最初の1件だけ残す。
			continue
		}
		if !isX8664ELF(probe.path) {
			continue
		}
		seen[probe.kind] = true
		evidences = append(evidences, LibcEvidence{Path: probe.path, Kind: probe.kind})
	}
	return evidences
}

// isX8664ELF はpathがx86-64のELF fileかどうかを返す。
//
// symlinkは辿る。loaderはdistributionによってsymlinkで置かれるため、実体を見るのが
// 目的に合う。
func isX8664ELF(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	header := make([]byte, elfIdentSize)
	if _, err := io.ReadFull(file, header); err != nil {
		// 短すぎるfileはELFではない。読取り失敗も同じく「根拠にしない」。
		return false
	}
	return isX8664ELFHeader(header)
}

// isX8664ELFHeader はELF headerの先頭がx86-64を示すかを返す。
//
// byte列だけを見る純粋な判定であり、両OSのtestで確かめる。
func isX8664ELFHeader(header []byte) bool {
	if len(header) < elfIdentSize {
		return false
	}
	if string(header[:len(elfMagic)]) != elfMagic {
		return false
	}
	if header[elfClassOffset] != elfClass64 || header[elfDataOffset] != elfDataLSB {
		return false
	}
	machine := uint16(header[elfMachineOffset]) | uint16(header[elfMachineOffset+1])<<8
	return machine == elfMachineX8664
}
