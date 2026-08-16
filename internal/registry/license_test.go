package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// pythonLicensePath はPython third-party licenseのregistry内pathである
// （docs/07-registry-and-tools.md §2）。
const pythonLicensePath = "licenses/python-build-standalone-MPL-2.0.txt"

// pythonLicenseUpstream はlicense textの取得元である。
//
// §2は「内容はupstream取得元とlicense identifierをregistry reviewで照合する」と
// 定める。取得元をtestへ書いておき、次に更新する人が同じ場所を見られるようにする。
const pythonLicenseUpstream = "https://raw.githubusercontent.com/astral-sh/python-build-standalone/main/LICENSE"

// TestRepositoryPythonLicenseExists は§5第7項「Python third-party license text
// が存在する」を固定する。
func TestRepositoryPythonLicenseExists(t *testing.T) {
	data := readLicense(t)
	if len(data) == 0 {
		t.Fatal("license textが空である")
	}
	// §5第2項のsize上限。docs/04-storage-and-data.md §21の「registry manifest
	// 各file 2 MiB」をregistry treeのfileへ適用する。
	if len(data) > MessageFileMaxBytes {
		t.Errorf("license textが%d byteを超える（%d byte）", MessageFileMaxBytes, len(data))
	}
}

// TestRepositoryPythonLicenseIdentity はlicense textが宣言した識別子の本文で
// あることを固定する。
//
// §2の「license identifierをregistry reviewで照合する」を機械的に再現できる形に
// する。`python.toml`のproviderは`MPL-2.0`を宣言しており、別のlicense本文へ
// 差し替わったことに気付けないと、Planが表示するlicenseと同梱本文が食い違う。
func TestRepositoryPythonLicenseIdentity(t *testing.T) {
	text := string(readLicense(t))

	// MPL-2.0の題名。SPDX識別子`MPL-2.0`に対応する正式名称である。
	if !strings.HasPrefix(text, "Mozilla Public License Version 2.0\n") {
		t.Fatalf("先頭がMPL-2.0の題名でない: %q", firstLine(text))
	}
	// 本文の節構成。題名だけの要約やdiffで欠けた本文を通さない。
	for _, section := range []string{
		"\n1. Definitions\n",
		"\n2. License Grants and Conditions\n",
		"\n3. Responsibilities\n",
		"\n4. Inability to Comply Due to Statute or Regulation\n",
		"\n5. Termination\n",
		"\n8. Litigation\n",
		"\n9. Miscellaneous\n",
		"\n10. Versions of the License\n",
		"\nExhibit A - Source Code Form License Notice\n",
		"\nExhibit B - \"Incompatible With Secondary Licenses\" Notice\n",
	} {
		if !strings.Contains(text, section) {
			t.Errorf("MPL-2.0の節 %q が無い", strings.TrimSpace(section))
		}
	}
}

// TestRepositoryPythonLicenseEncoding はlicense textの表現を固定する。
//
// registryはWindowsとLinuxの両方へ同じbytesで配られ、digestも同じでなければ
// ならない。CRLFやBOMが混ざるとplatformごとにbytesが変わる。
func TestRepositoryPythonLicenseEncoding(t *testing.T) {
	data := readLicense(t)
	text := string(data)

	if !utf8.Valid(data) {
		t.Fatal("UTF-8として不正である")
	}
	// Go sourceへBOMのliteralは書けないため、escapeで書く。
	if strings.HasPrefix(text, "\uFEFF") {
		t.Error("BOMがある")
	}
	if strings.Contains(text, "\r") {
		t.Error("CRがある。改行はLFだけにする")
	}
	if !strings.HasSuffix(text, "\n") {
		t.Error("末尾がLFで終わっていない")
	}
	for index, r := range text {
		if r > 0x7F {
			t.Fatalf("%d byte目に非ASCII文字 U+%04X がある", index, r)
		}
	}
}

func readLicense(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(registryDir, pythonLicensePath))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v。取得元は %s", pythonLicensePath, err, pythonLicenseUpstream)
	}
	return data
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}
