// Package repair reads what DISM /RestoreHealth and sfc /scannow report.
//
// Both tools only have a text and an exit code to go on, and the text depends
// on the Windows language. The messages below were read from the tools' own
// message files on Windows 11 (sfc.exe.mui and CbsProvider.dll.mui, en-US and
// pt-BR). Anything not recognized is reported as such: the Axion never
// guesses a good result out of text it does not understand.
package repair

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Outcome is what one tool reported.
type Outcome string

const (
	OK         Outcome = "ok"         // finished and found nothing to fix (or DISM finished without error)
	Repaired   Outcome = "repaired"   // SFC fixed damaged files
	Reboot     Outcome = "reboot"     // finished or queued, a restart is needed to complete
	Unrepaired Outcome = "unrepaired" // damage found that the tool could not fix
	Failed     Outcome = "failed"     // the tool could not do the work
	Unknown    Outcome = "unknown"    // exit code 0 but a message the Axion does not recognize
	Cancelled  Outcome = "cancelled"  // stopped by the user
	Skipped    Outcome = "skipped"    // not run
)

// ExitRebootRequired is ERROR_SUCCESS_REBOOT_REQUIRED, which DISM returns when
// it finished and a restart completes the work.
const ExitRebootRequired = 3010

// cbsNote points to where the SFC details are kept.
const cbsNote = ` Os detalhes estão em C:\Windows\Logs\CBS\CBS.log.`

var folds = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "\u00a0", " ",
)

// Fold lowercases text, drops accents (and any other non-ASCII character a
// wrong console code page may have produced) and squeezes white space, so a
// message matches in pt-BR whatever the accents came out as.
func Fold(s string) string {
	s = folds.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		default:
			// dropped
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func has(folded string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(folded, p) {
			return false
		}
	}
	return true
}

func anyOf(folded string, opts ...string) bool {
	for _, p := range opts {
		if strings.Contains(folded, p) {
			return true
		}
	}
	return false
}

var pctRe = regexp.MustCompile(`(\d{1,3}(?:[.,]\d)?)\s*%`)

// Percent extracts the progress of a DISM bar ("[===  45.0%  ]") or of an SFC
// line ("Verification 45% complete.", "Verificação 45% concluída.").
func Percent(line string) (float64, bool) {
	m := pctRe.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	if err != nil || f < 0 || f > 100 {
		return 0, false
	}
	return f, true
}

// ExitText shows an exit code the way DISM prints it: an HRESULT in hex.
func ExitText(code int) string {
	if code < 0 || code > 0xFFFF {
		return fmt.Sprintf("0x%08X", uint32(code))
	}
	return strconv.Itoa(code)
}

// ParseDISM reads a finished "DISM /Online /Cleanup-Image /RestoreHealth".
func ParseDISM(exit int, output string) (Outcome, string) {
	t := Fold(output)
	switch {
	case exit == 0:
		return OK, "O repositório de componentes do Windows foi verificado e o DISM terminou sem erros."
	case exit == ExitRebootRequired:
		return Reboot, "O DISM terminou, mas o Windows pede um reinício para concluir."
	case exit == 740 || anyOf(t, "elevated command prompt", "privilegios elevados"):
		return Failed, "O DISM exige o Axion como administrador."
	case anyOf(t, "currently performing another servicing operation", "executando outra operacao de manutencao"):
		return Failed, "O Windows está fazendo outra manutenção agora (por exemplo, instalando uma atualização). Espere terminar e tente de novo."
	case anyOf(t, "pending operations", "operacoes pendentes", "pending restart", "reinicializacao pendente"):
		return Failed, "Há operações pendentes no Windows. Reinicie o PC e tente de novo."
	case anyOf(t, "could not be downloaded", "nao foi possivel fazer o download"):
		return Failed, "O DISM não conseguiu baixar os arquivos de origem do Windows Update. Confira a conexão com a internet e tente de novo."
	case anyOf(t, "source files could not be found", "repair content could not be found", "arquivos de origem nao puderam ser encontrados", "nao foi possivel encontrar os arquivos de origem", "conteudo de reparo nao pode ser encontrado", "fonte do reparo nao foi encontrada"):
		return Failed, "O DISM não encontrou os arquivos originais para reparar (o Windows Update é a origem padrão). Confira a conexão com a internet e tente de novo."
	}
	return Failed, "O DISM terminou com o código " + ExitText(exit) + "."
}

// ParseSFC reads a finished "sfc /scannow".
func ParseSFC(exit int, output string) (Outcome, string) {
	t := Fold(output)
	switch {
	case anyOf(t, "could not start the repair service", "nao pode iniciar o servico de reparo"):
		return Failed, "O Windows não conseguiu iniciar o serviço de reparo. Reinicie o PC e tente de novo."
	case anyOf(t, "another servicing or repair operation", "outro servico ou operacao de reparo"):
		return Failed, "Outra manutenção do Windows está em andamento. Espere terminar e tente de novo."
	case anyOf(t, "system repair pending", "reparo de sistema pendente"):
		return Reboot, "Há um reparo pendente: reinicie o PC e rode o reparo de novo."
	case anyOf(t, "could not perform the requested operation", "nao pode executar a operacao solicitada"):
		return Failed, "O Windows não conseguiu fazer a verificação dos arquivos de sistema. Reinicie o PC e tente de novo."
	case has(t, "corrupt") && anyOf(t, "but was unable to fix", "mas nao conseguiu corrigir", "mas nao pode corrigir"),
		has(t, "arquivos corrompidos") && anyOf(t, "mas nao conseguiu", "mas nao pode"):
		return Unrepaired, "O SFC achou arquivos de sistema danificados e não conseguiu corrigir alguns." + cbsNote
	case anyOf(t, "queued for repair", "na fila para reparo", "take effect after the next reboot", "em vigor apos a proxima reinicializacao"):
		return Reboot, "O SFC deixou arquivos na fila para reparar: reinicie o PC para concluir e rode o reparo de novo."
	case has(t, "corrupt files and successfully repaired") || has(t, "arquivos corrompidos e os reparou"):
		return Repaired, "O SFC achou arquivos de sistema danificados e os repôs com sucesso." + cbsNote
	case has(t, "did not find any integrity violations but cannot") || has(t, "violacoes de integridade", "mas nao pode"):
		return Unrepaired, "O SFC não achou arquivos danificados, mas avisou que não garante a integridade do sistema porque os metadados dos componentes estão corrompidos. Rode o reparo de novo; se repetir, o problema está no repositório de componentes."
	case anyOf(t, "did not find any integrity violations", "nao encontrou nenhuma violacao de integridade"):
		return OK, "O SFC verificou os arquivos de sistema protegidos e não achou nenhum danificado."
	case exit != 0:
		return Failed, "O SFC terminou com o código " + ExitText(exit) + "."
	}
	return Unknown, "O SFC terminou sem erro, mas o Axion não reconheceu a mensagem final (talvez o Windows esteja em outro idioma). Veja os detalhes técnicos."
}
