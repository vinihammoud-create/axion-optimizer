package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// The winget side of the catalog: the commands the Axion runs, what their
// output and exit codes mean. winget ("App Installer") ships with Windows
// 10 1809+ and Windows 11; every command here accepts the source and
// package agreements up front and runs without prompts.

// Common arguments of every call: no prompts, agreements accepted.
var commonArgs = []string{"--accept-source-agreements", "--disable-interactivity"}

// InstallArgs builds the install command of an item.
func InstallArgs(it Item) []string {
	switch {
	case it.Store != "":
		return append([]string{"install", "--id", it.Store, "-e", "-s", "msstore", "--accept-package-agreements"}, commonArgs...)
	case it.Winget != "":
		return append([]string{"install", "--id", it.Winget, "-e", "--silent", "--accept-package-agreements"}, commonArgs...)
	}
	return nil
}

// UpgradeArgs builds the upgrade command of an item.
func UpgradeArgs(it Item) []string {
	switch {
	case it.Store != "":
		return append([]string{"upgrade", "--id", it.Store, "-e", "-s", "msstore", "--accept-package-agreements"}, commonArgs...)
	case it.Winget != "":
		return append([]string{"upgrade", "--id", it.Winget, "-e", "--silent", "--accept-package-agreements"}, commonArgs...)
	}
	return nil
}

// UpgradeAllArgs upgrades every package winget knows a newer version of
// (packages whose installed version it cannot read are left alone).
func UpgradeAllArgs() []string {
	return append([]string{"upgrade", "--all", "--silent", "--accept-package-agreements"}, commonArgs...)
}

// ExportArgs writes the installed packages winget recognises to a JSON
// file (the way to read "what is installed" without parsing tables).
func ExportArgs(path string) []string {
	return append([]string{"export", "-o", path, "--include-versions"}, commonArgs...)
}

// UpgradeListArgs lists the packages with a newer version available.
func UpgradeListArgs() []string {
	return append([]string{"upgrade"}, commonArgs...)
}

// Installed is one package winget found installed.
type Installed struct {
	ID        string `json:"id"`
	Version   string `json:"version,omitempty"`
	Available string `json:"available,omitempty"` // newer version in the catalog
}

// ParseExport reads the JSON winget export writes: the identifiers (and
// versions) of the installed packages it matched to a source.
func ParseExport(b []byte) (map[string]Installed, error) {
	var doc struct {
		Sources []struct {
			Packages []struct {
				PackageIdentifier string `json:"PackageIdentifier"`
				Version           string `json:"Version"`
			} `json:"Packages"`
		} `json:"Sources"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &doc); err != nil {
		return nil, err
	}
	out := map[string]Installed{}
	for _, s := range doc.Sources {
		for _, p := range s.Packages {
			if id := strings.TrimSpace(p.PackageIdentifier); id != "" {
				out[strings.ToLower(id)] = Installed{ID: id, Version: cleanVersion(p.Version)}
			}
		}
	}
	return out, nil
}

// Identifiers as the two sources print them: a winget id is
// Publisher.Name (dots, no spaces, never cut with "…"); a Store product id
// is 12 upper-case letters and digits.
var (
	wingetIDRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_-]*(\.[A-Za-z0-9+_-]+)+$`)
	msstoreIDRe = regexp.MustCompile(`^[0-9A-Z]{12}$`)
)

// cleanVersion drops the placeholders winget prints when it cannot read
// the installed version.
func cleanVersion(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "unknown", "desconhecido", "desconhecida", "":
		return ""
	}
	return strings.TrimSpace(v)
}

// ParseUpgradeTable reads the table "winget upgrade" prints (Name, Id,
// Version, Available, Source — the headers are localized, the layout is
// not): the identifier and the available version of each row. The name
// may carry spaces and be cut with "…", so the row is read from its end,
// where the four last fields have no spaces. Rows whose identifier is cut
// ("…") or does not look like a package id (a note whose last word is the
// source name) are dropped; a version winget does not know is kept empty.
func ParseUpgradeTable(out string) map[string]Installed {
	res := map[string]Installed{}
	lines := strings.Split(strings.ReplaceAll(out, "\r", ""), "\n")
	inTable := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "---") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		f := strings.Fields(t)
		if len(f) < 5 {
			continue // a row has at least a name and the four fields
		}
		src := f[len(f)-1]
		avail, ver, id := f[len(f)-2], f[len(f)-3], f[len(f)-4]
		switch src {
		case "winget":
			if !wingetIDRe.MatchString(id) {
				continue
			}
		case "msstore":
			if !msstoreIDRe.MatchString(id) {
				continue
			}
		default:
			continue // the trailer ("N upgrades available.") or a note
		}
		if strings.ContainsAny(avail, "…<>") || strings.Contains(strings.ToLower(avail), "unknown") {
			continue
		}
		res[strings.ToLower(id)] = Installed{ID: id, Version: cleanVersion(ver), Available: avail}
	}
	return res
}

// Return codes of winget (doc/windows/package-manager/winget/returnCodes.md
// of the winget-cli repository), as the process exit code (32-bit).
const (
	CodeOK               = 0
	CodeBadArgs          = 0x8A150002 // "Invalid command line arguments" (an App Installer too old for a flag)
	CodeAlreadyInstalled = 0x8A150061 // "Found at least one version of the package installed."
	CodeNoUpdate         = 0x8A15002B // "No applicable update found"
	CodeUpgradeAllFail   = 0x8A15002C // "winget upgrade --all completed with failures"
	CodeNoPackage        = 0x8A150014 // "No packages found"
	CodeNoInstaller      = 0x8A150010 // "None of the installers are applicable for the current system"
	CodeShellExecute     = 0x8A150006 // "Running ShellExecute failed": the exe installer ran and returned an error
	CodeMSI              = 0x8A150049 // "Running MSI install failed"
	CodeDownload         = 0x8A150008 // "Downloading installer failed"
	CodeHash             = 0x8A150011 // "The installer file's hash does not match the manifest"
	CodeSecurityCheck    = 0x8A15002D // "Installer failed security check"
	CodeSizeMismatch     = 0x8A15002E // "Download size does not match expected content length"
	CodeStoreFailed      = 0x8A15001E // "Failed to install the Microsoft Store app"
	CodeStoreBlocked     = 0x8A15001B // "The Microsoft Store client is blocked by policy"
	CodeStoreAppBlocked  = 0x8A15001C // "The Microsoft Store app is blocked by policy"
	CodePkgAgreements    = 0x8A150041
	CodeSrcAgreements    = 0x8A150046
	CodeNoElevation      = 0x8A150056 // "The installer cannot be run from an administrator context."
	CodePinned           = 0x8A150068 // "The package has a pin that prevents upgrade."
	CodeTechMismatch     = 0x8A15008E // "An upgrade is available but uses a different install technology"
	CodeAppRunning       = 0x8A150101 // "Application is currently running. Exit the application then try again."
	CodeOtherInstall     = 0x8A150102 // "Another installation is already in progress. Try again later."
	CodeFileInUse        = 0x8A150103 // "One or more file is being used. Exit the application then try again."
	CodeDiskFull         = 0x8A150105 // "There's no more space on your PC. Make space, then try again."
	CodeNoNetwork        = 0x8A150107 // "This application requires internet connectivity."
	CodeRebootRequired   = 0x8A150109 // "Restart your PC to finish installation."
	CodeRebootFirst      = 0x8A15010A // "Installation failed. Restart your PC then try again."
	CodeRebootInitiated  = 0x8A15010B // "Your PC will restart to finish installation."
	CodeCancelled        = 0x8A15010C // "You cancelled the installation."
	CodeOtherVersion     = 0x8A15010D // "Another version of this application is already installed."
	CodeDowngrade        = 0x8A15010E // "A higher version of this application is already installed."
	CodePolicyBlocked    = 0x8A15010F // "Organization policies are preventing installation."
	CodeInUseByApp       = 0x8A150111 // "Application is currently in use by another application."
	CodeNotSupported     = 0x8A150113 // "Package not supported by the system."
	CodeNoUpgradeSupport = 0x8A150114 // "The installer does not support upgrading an existing package."
	CodeCustomError      = 0x8A150115 // "Installation failed with a custom installer error."
)

// Outcome classifies an exit code.
type Outcome struct {
	Code    uint32 `json:"code"`
	OK      bool   `json:"ok"`      // installed, upgraded or already there
	Reboot  bool   `json:"reboot"`  // done, but a restart finishes it
	Skipped bool   `json:"skipped"` // nothing to do (already installed, no update)
	Partial bool   `json:"partial"` // upgrade --all: some packages failed, the others went
	Text    string `json:"text"`
}

// Classify explains an exit code. The output's last line is winget's own
// description of the failure, kept whenever the code says the installer
// itself failed (its exit code is in that line) or the code is unknown.
func Classify(exit int, lastLine string) Outcome {
	code := uint32(exit)
	withLine := func(t string) string {
		if lastLine != "" {
			return t + " (" + lastLine + ")"
		}
		return t
	}
	switch code {
	case CodeOK:
		return Outcome{Code: code, OK: true, Text: "concluído"}
	case CodeAlreadyInstalled:
		return Outcome{Code: code, OK: true, Skipped: true, Text: "já estava instalado"}
	case CodeNoUpdate:
		return Outcome{Code: code, OK: true, Skipped: true, Text: "já está na versão mais nova"}
	case CodeDowngrade:
		return Outcome{Code: code, OK: true, Skipped: true, Text: "uma versão mais nova já está instalada"}
	case CodeRebootRequired:
		return Outcome{Code: code, OK: true, Reboot: true, Text: "instalado; reinicie o PC para concluir"}
	case CodeRebootInitiated:
		return Outcome{Code: code, OK: true, Reboot: true, Text: "instalado; o instalador pediu para reiniciar o PC"}
	case CodeUpgradeAllFail:
		return Outcome{Code: code, Partial: true, Text: "parte das atualizações falhou; os detalhes estão na resposta do winget"}
	case CodeBadArgs:
		return Outcome{Code: code, Text: "o Instalador de Aplicativo deste Windows é antigo e não conhece as opções usadas: atualize-o pela Microsoft Store (aka.ms/getwinget)"}
	case CodeNoPackage:
		return Outcome{Code: code, Text: "pacote não encontrado (não está no catálogo do winget, ou não está instalado para atualizar)"}
	case CodeNoInstaller:
		return Outcome{Code: code, Text: "nenhum instalador do pacote serve para este Windows"}
	case CodeShellExecute, CodeMSI, CodeCustomError:
		return Outcome{Code: code, Text: withLine("o instalador do programa rodou e terminou com erro")}
	case CodeDownload, CodeSizeMismatch:
		return Outcome{Code: code, Text: "o download do instalador falhou (rede ou servidor do fabricante)"}
	case CodeHash:
		return Outcome{Code: code, Text: "o instalador baixado não confere com o hash do catálogo; nada foi instalado"}
	case CodeSecurityCheck:
		return Outcome{Code: code, Text: "o instalador não passou na verificação de segurança do winget; nada foi instalado"}
	case CodeStoreFailed:
		return Outcome{Code: code, Text: withLine("a instalação pela Microsoft Store falhou")}
	case CodeStoreBlocked, CodeStoreAppBlocked, CodePolicyBlocked:
		return Outcome{Code: code, Text: "bloqueado por política do Windows (Microsoft Store ou instalação)"}
	case CodePkgAgreements, CodeSrcAgreements:
		return Outcome{Code: code, Text: "o winget pediu aceite de termos que o Axion não pôde dar"}
	case CodeNoElevation:
		return Outcome{Code: code, Text: "o instalador deste programa não aceita rodar como administrador (o Axion roda elevado): instale-o pela Microsoft Store ou pelo site"}
	case CodePinned:
		return Outcome{Code: code, Text: "o pacote está fixado (winget pin) e não é atualizado"}
	case CodeTechMismatch, CodeNoUpgradeSupport:
		return Outcome{Code: code, Text: "a versão nova usa outro tipo de instalador: desinstale a atual e instale de novo"}
	case CodeAppRunning, CodeFileInUse, CodeInUseByApp:
		return Outcome{Code: code, Text: "o programa está aberto: feche-o e tente de novo"}
	case CodeOtherInstall:
		return Outcome{Code: code, Text: "outra instalação em andamento no Windows; tente de novo em instantes"}
	case CodeDiskFull:
		return Outcome{Code: code, Text: "sem espaço em disco"}
	case CodeNoNetwork:
		return Outcome{Code: code, Text: "sem internet"}
	case CodeRebootFirst:
		return Outcome{Code: code, Text: "o instalador pediu para reiniciar o PC antes de tentar de novo"}
	case CodeCancelled:
		return Outcome{Code: code, Text: "instalação cancelada"}
	case CodeOtherVersion:
		return Outcome{Code: code, Text: "outra versão do programa já está instalada e o instalador não a substitui"}
	case CodeNotSupported:
		return Outcome{Code: code, Text: "o pacote não serve para este Windows"}
	}
	t := fmt.Sprintf("o winget terminou com o código 0x%08X", code)
	if lastLine != "" {
		t += ": " + lastLine
	}
	return Outcome{Code: code, Text: t}
}

// LastLine is the last non-empty line of an output.
func LastLine(out string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\r", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" && !strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "\\") && !strings.HasPrefix(l, "|") && !strings.HasPrefix(l, "/") {
			return l
		}
	}
	return ""
}
