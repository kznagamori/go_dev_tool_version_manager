package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// 判定（ClassifyHost）は純粋な変換であり、observationを組み立てれば両OSの実行で
// musl・arm64・未知kernelを網羅して確かめられる。実機を用意する必要がない。

func glibcEvidence() []LibcEvidence {
	return []LibcEvidence{{Path: "/lib64/ld-linux-x86-64.so.2", Kind: LibcKindGlibc}}
}

func muslEvidence() []LibcEvidence {
	return []LibcEvidence{{Path: "/lib/ld-musl-x86_64.so.1", Kind: LibcKindMusl}}
}

func TestClassifyHostAcceptsSupportedHosts(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		facts HostFacts
		want  string
	}{
		"Linux/x86_64/glibc": {
			facts: HostFacts{KernelOS: "Linux", KernelArch: "x86_64", LibcEvidences: glibcEvidence()},
			want:  domain.PlatformLinuxAMD64Glibc,
		},
		"Windows/x86_64": {
			facts: HostFacts{KernelOS: "windows", KernelArch: "x86_64"},
			want:  domain.PlatformWindowsAMD64,
		},
		// uname(2)の値はhostでcaseと前後空白が揺れる。表を増やさず入力を整える。
		"case・空白の揺れ": {
			facts: HostFacts{KernelOS: " LINUX ", KernelArch: "X86_64 ", LibcEvidences: glibcEvidence()},
			want:  domain.PlatformLinuxAMD64Glibc,
		},
		"machine名がamd64表記": {
			facts: HostFacts{KernelOS: "linux", KernelArch: "amd64", LibcEvidences: glibcEvidence()},
			want:  domain.PlatformLinuxAMD64Glibc,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			platform, err := ClassifyHost(tc.facts)
			if err != nil {
				t.Fatalf("ClassifyHost: %v", err)
			}
			if platform.ID() != tc.want {
				t.Errorf("platform = %q, want %q", platform.ID(), tc.want)
			}
		})
	}
}

func TestClassifyHostRejectsUnsupportedHosts(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		facts HostFacts
		// wantIn はerror文へ必ず現れる語である。利用者が「何が対象外だったか」を
		// 読み取れることまで固定する（docs/09-platform.md §9「errorには対象…と
		// 利用者が選べる安全な代替を含める」）。
		wantIn string
	}{
		// docs/09-platform.md §1「arm64、Linux musl等は…非対応」。
		"arm64 kernel": {
			facts:  HostFacts{KernelOS: "linux", KernelArch: "aarch64", LibcEvidences: glibcEvidence()},
			wantIn: "aarch64",
		},
		"Windows on ARM": {
			facts:  HostFacts{KernelOS: "windows", KernelArch: "arm64"},
			wantIn: "arm64",
		},
		"32 bit x86": {
			facts:  HostFacts{KernelOS: "linux", KernelArch: "i686", LibcEvidences: glibcEvidence()},
			wantIn: "i686",
		},
		"musl": {
			facts:  HostFacts{KernelOS: "linux", KernelArch: "x86_64", LibcEvidences: muslEvidence()},
			wantIn: "musl",
		},
		// **glibcが見つからないことをglibcと見なさない。**
		"libc観測なし": {
			facts:  HostFacts{KernelOS: "linux", KernelArch: "x86_64"},
			wantIn: "glibc",
		},
		// gcompatを入れたAlpineや多重libcのcontainer。片方に決めて進めない。
		"glibcとmuslが同時": {
			facts: HostFacts{
				KernelOS:      "linux",
				KernelArch:    "x86_64",
				LibcEvidences: append(glibcEvidence(), muslEvidence()...),
			},
			wantIn: "一意に決められない",
		},
		"未知のkernel": {
			facts:  HostFacts{KernelOS: "Darwin", KernelArch: "x86_64"},
			wantIn: "Darwin",
		},
		"kernel名が空": {
			facts:  HostFacts{KernelOS: "", KernelArch: "x86_64"},
			wantIn: "対象外",
		},
		"machine名が空": {
			facts:  HostFacts{KernelOS: "linux", KernelArch: "", LibcEvidences: glibcEvidence()},
			wantIn: "対象外",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			platform, err := ClassifyHost(tc.facts)
			if err == nil {
				t.Fatalf("対象外hostが通った: platform = %q", platform.ID())
			}
			if err.Code != domain.CodePlatformUnsupported {
				t.Errorf("code = %q, want %q", err.Code, domain.CodePlatformUnsupported)
			}
			// retryしても解消しない。retryさせると利用者を待たせるだけである。
			if err.Retryable {
				t.Error("Retryable = true, 対象外hostは再試行で解消しない")
			}
			// 理由はCauseが運ぶ。domain.Error.Error()はCauseを含めない規約であり
			// （internal/domain/error.go）、診断はCauseと構造化logから辿る。
			if err.Cause == nil {
				t.Fatal("Causeが無い。何が対象外だったかを診断できない")
			}
			if !strings.Contains(err.Cause.Error(), tc.wantIn) {
				t.Errorf("cause = %v, %q を含むべき", err.Cause, tc.wantIn)
			}
			if !platform.IsZero() {
				t.Errorf("拒否したのにplatform = %q が返った", platform.ID())
			}
		})
	}
}

func TestClassifyHostIgnoresLibcOnWindows(t *testing.T) {
	t.Parallel()
	// WindowsにC標準libraryの選択は無い。仮にloaderらしきものが観測されても
	// 判定へ持ち込まない。
	platform, err := ClassifyHost(HostFacts{
		KernelOS: "windows", KernelArch: "x86_64", LibcEvidences: muslEvidence(),
	})
	if err != nil {
		t.Fatalf("ClassifyHost: %v", err)
	}
	if platform.ID() != domain.PlatformWindowsAMD64 {
		t.Errorf("platform = %q, want %q", platform.ID(), domain.PlatformWindowsAMD64)
	}
	if platform.Libc() != domain.LibcNone {
		t.Errorf("libc = %q, want %q", platform.Libc(), domain.LibcNone)
	}
}

func TestClassifyHostErrorNamesEveryObservation(t *testing.T) {
	t.Parallel()
	// `doctor`が「何を見て対象外と判定したか」を示せること。観測をerror文から
	// 落とすと、利用者はloaderを1件ずつ自分で調べることになる。
	facts := HostFacts{
		KernelOS:   "linux",
		KernelArch: "x86_64",
		LibcEvidences: []LibcEvidence{
			{Path: "/lib/ld-musl-x86_64.so.1", Kind: LibcKindMusl},
			{Path: "/lib64/ld-linux-x86-64.so.2", Kind: LibcKindGlibc},
		},
	}
	_, err := ClassifyHost(facts)
	if err == nil {
		t.Fatal("glibcとmuslの同時観測が通った")
	}
	if err.Cause == nil {
		t.Fatal("Causeが無い。観測を診断へ運べない")
	}
	for _, evidence := range facts.LibcEvidences {
		if !strings.Contains(err.Cause.Error(), evidence.Path) {
			t.Errorf("cause = %v, 観測 %q を含むべき", err.Cause, evidence.Path)
		}
	}
}

func TestDescribeEvidenceIsStable(t *testing.T) {
	t.Parallel()
	// 観測順はfilesystemの都合で変わりうる。error文が実行ごとに変わると、
	// 利用者報告どうしを突き合わせられない。
	forward := describeEvidence([]LibcEvidence{
		{Path: "/b", Kind: LibcKindGlibc},
		{Path: "/a", Kind: LibcKindMusl},
	})
	reverse := describeEvidence([]LibcEvidence{
		{Path: "/a", Kind: LibcKindMusl},
		{Path: "/b", Kind: LibcKindGlibc},
	})
	if forward != reverse {
		t.Errorf("順序で結果が変わる: %q vs %q", forward, reverse)
	}
	if describeEvidence(nil) != "観測なし" {
		t.Errorf("空の観測 = %q, want %q", describeEvidence(nil), "観測なし")
	}
}

// ELF identityの判定はbyte列だけを見る純粋な変換であり、両OSのtestで確かめる。

// elfHeader はtest用のELF headerを組み立てる。
func elfHeader(class, data byte, machine uint16) []byte {
	header := make([]byte, elfIdentSize)
	copy(header, elfMagic)
	header[elfClassOffset] = class
	header[elfDataOffset] = data
	header[elfMachineOffset] = byte(machine)
	header[elfMachineOffset+1] = byte(machine >> 8)
	return header
}

func TestIsX8664ELFHeader(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		header []byte
		want   bool
	}{
		"x86-64":        {header: elfHeader(elfClass64, elfDataLSB, elfMachineX8664), want: true},
		"aarch64":       {header: elfHeader(elfClass64, elfDataLSB, 0xB7), want: false},
		"32 bit":        {header: elfHeader(1, elfDataLSB, elfMachineX8664), want: false},
		"big endian":    {header: elfHeader(elfClass64, 2, elfMachineX8664), want: false},
		"header不足":      {header: elfHeader(elfClass64, elfDataLSB, elfMachineX8664)[:elfIdentSize-1], want: false},
		"空":             {header: nil, want: false},
		"magicが違う":      {header: append([]byte("NOPE"), make([]byte, elfIdentSize-4)...), want: false},
		"magic 1 byte差": {header: append([]byte("\x7fELX"), elfHeader(elfClass64, elfDataLSB, elfMachineX8664)[4:]...), want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := isX8664ELFHeader(tc.header); got != tc.want {
				t.Errorf("isX8664ELFHeader = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsX8664ELFReadsRealFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	valid := filepath.Join(root, "loader.so")
	if err := os.WriteFile(valid, elfHeader(elfClass64, elfDataLSB, elfMachineX8664), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !isX8664ELF(valid) {
		t.Error("x86-64 ELFをELFと判定しなかった")
	}

	// **path名だけで決めない。** 名前がloaderらしいだけの別fileを根拠にすると、
	// 対象外hostを対象と誤判定する。
	fake := filepath.Join(root, "ld-linux-x86-64.so.2")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho not a loader\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if isX8664ELF(fake) {
		t.Error("ELFでないfileをloaderと判定した")
	}

	// 短すぎるfileはheaderを読めない。読取り失敗を根拠にしない。
	short := filepath.Join(root, "short.so")
	if err := os.WriteFile(short, []byte(elfMagic), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if isX8664ELF(short) {
		t.Error("header不足のfileをloaderと判定した")
	}

	if isX8664ELF(filepath.Join(root, "missing.so")) {
		t.Error("存在しないpathをloaderと判定した")
	}
	// directoryは読めない。falseで返り、panicしない。
	if isX8664ELF(root) {
		t.Error("directoryをloaderと判定した")
	}
}

func TestProbeLibcCollectsFirstEvidencePerKind(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	header := elfHeader(elfClass64, elfDataLSB, elfMachineX8664)

	first := filepath.Join(root, "glibc-a.so")
	second := filepath.Join(root, "glibc-b.so")
	musl := filepath.Join(root, "musl.so")
	for _, path := range []string{first, second, musl} {
		if err := os.WriteFile(path, header, 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	evidences := probeLibc([]libcProbe{
		{path: filepath.Join(root, "missing.so"), kind: LibcKindGlibc},
		{path: first, kind: LibcKindGlibc},
		{path: second, kind: LibcKindGlibc},
		{path: musl, kind: LibcKindMusl},
	})
	// 同じlibcの根拠を重ねても判定は変わらない。error文が長くなるだけである。
	if len(evidences) != 2 {
		t.Fatalf("evidences = %+v, 2件であるべき", evidences)
	}
	if evidences[0].Path != first || evidences[0].Kind != LibcKindGlibc {
		t.Errorf("1件目 = %+v, want %q/%q", evidences[0], first, LibcKindGlibc)
	}
	if evidences[1].Path != musl || evidences[1].Kind != LibcKindMusl {
		t.Errorf("2件目 = %+v, want %q/%q", evidences[1], musl, LibcKindMusl)
	}
	// 観測できなければ空で返る。nilを返して呼出し側で分岐させない。
	if got := probeLibc(nil); len(got) != 0 {
		t.Errorf("probeLibc(nil) = %+v, 空であるべき", got)
	}
}

func TestDetectHostMatchesThisMachine(t *testing.T) {
	t.Parallel()
	// CI matrixの両OSで実行する。ここが通ることが、uname(2)／
	// GetNativeSystemInfoの呼出しとlinuxLibcProbesの表が実環境に合っている証拠に
	// なる。合わなければ対象hostが対象外と判定される。
	host, err := DetectHost()
	if err != nil {
		t.Fatalf("DetectHost: %v（観測: %+v）", err, host.Facts)
	}
	if host.Platform.IsZero() {
		t.Fatal("platformが零値である")
	}
	if host.Facts.KernelOS == "" {
		t.Error("KernelOSが空である")
	}
	if host.Facts.KernelArch == "" {
		t.Error("KernelArchが空である")
	}
	switch host.Platform.OS() {
	case domain.OSLinux:
		if host.Platform.Libc() != domain.LibcGlibc {
			t.Errorf("libc = %q, want %q", host.Platform.Libc(), domain.LibcGlibc)
		}
		if len(host.Facts.LibcEvidences) == 0 {
			t.Error("glibcと判定したのに観測が空である")
		}
		if !host.Facts.LongPathsEnabled {
			t.Error("LinuxでLongPathsEnabled = false")
		}
	case domain.OSWindows:
		if host.Platform.Libc() != domain.LibcNone {
			t.Errorf("libc = %q, want %q", host.Platform.Libc(), domain.LibcNone)
		}
		if len(host.Facts.LibcEvidences) != 0 {
			t.Errorf("Windowsでlibcを観測している: %+v", host.Facts.LibcEvidences)
		}
	default:
		t.Fatalf("想定しないOS: %q", host.Platform.OS())
	}
}

func TestDetectHostIsDeterministic(t *testing.T) {
	t.Parallel()
	// 同じhostで結果が変わると、setup stateへ保存したplatformと次回起動の判定が
	// 食い違う。
	first, err := DetectHost()
	if err != nil {
		t.Fatalf("DetectHost: %v", err)
	}
	second, err := DetectHost()
	if err != nil {
		t.Fatalf("DetectHost(2回目): %v", err)
	}
	if first.Platform.ID() != second.Platform.ID() {
		t.Errorf("platform = %q / %q", first.Platform.ID(), second.Platform.ID())
	}
	if first.Facts.KernelArch != second.Facts.KernelArch {
		t.Errorf("KernelArch = %q / %q", first.Facts.KernelArch, second.Facts.KernelArch)
	}
	if len(first.Facts.LibcEvidences) != len(second.Facts.LibcEvidences) {
		t.Errorf("観測件数 = %d / %d",
			len(first.Facts.LibcEvidences), len(second.Facts.LibcEvidences))
	}
}
