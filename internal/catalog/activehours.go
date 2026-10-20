package catalog

import (
	"fmt"
	"strconv"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Active hours of Windows Update (0.7): the window in which Windows does
// not restart the PC for an update. The hours are the user's (the playing
// or streaming hours), so the item is built on demand from its ID,
// "activehours:<start>-<end>", like the per-game profiles.

// KeyActiveHours is where the Settings app keeps the window.
const KeyActiveHours = `HKLM\SOFTWARE\Microsoft\WindowsUpdate\UX\Settings`

// ActiveHoursMaxSpan is the longest window Windows accepts (18 hours).
const ActiveHoursMaxSpan = 18

var srcActiveHours = tweaks.Source{Title: "Microsoft Learn — Política de horário ativo do Windows Update (ActiveHoursStart/ActiveHoursEnd, 0-23; janela de até 18 h; o Windows não reinicia para atualizar dentro dela)", URL: "https://learn.microsoft.com/windows/client-management/mdm/policy-csp-update#activehoursstart"}

// ActiveHoursID names the item for a window.
func ActiveHoursID(start, end int) string { return fmt.Sprintf("activehours:%d-%d", start, end) }

// ParseActiveHoursID reads the window out of an ID.
func ParseActiveHoursID(id string) (start, end int, ok bool) {
	rest, found := strings.CutPrefix(id, "activehours:")
	if !found {
		return 0, 0, false
	}
	a, b, found := strings.Cut(rest, "-")
	if !found {
		return 0, 0, false
	}
	start, err1 := strconv.Atoi(a)
	end, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || !ValidActiveHours(start, end) {
		return 0, 0, false
	}
	return start, end, true
}

// ValidActiveHours tells whether a window is one Windows accepts: hours
// 0-23, from 1 to 18 hours long (wrapping past midnight is fine).
func ValidActiveHours(start, end int) bool {
	if start < 0 || start > 23 || end < 0 || end > 23 || start == end {
		return false
	}
	span := (end - start + 24) % 24
	return span >= 1 && span <= ActiveHoursMaxSpan
}

// ActiveHoursSpan is the length of the window in hours.
func ActiveHoursSpan(start, end int) int { return (end - start + 24) % 24 }

// ActiveHoursProvider builds the item for a window on demand.
func ActiveHoursProvider(id string, f *facts.Facts) *tweaks.Tweak {
	start, end, ok := ParseActiveHoursID(id)
	if !ok {
		return nil
	}
	return &tweaks.Tweak{
		ID: id, Category: "update", Title: fmt.Sprintf("Horário ativo do Windows Update: das %02d:00 às %02d:00", start, end),
		Summary: "Dentro do horário ativo o Windows não reinicia o PC para instalar atualizações: coloque nele as horas em que você joga, transmite ou trabalha.",
		Detail: fmt.Sprintf("ActiveHoursStart = %d e ActiveHoursEnd = %d (REG_DWORD, horas de 0 a 23) em %s, os mesmos valores que Configurações › Windows Update › Opções avançadas › Horário ativo grava. A janela tem no máximo %d horas. ", start, end, KeyActiveHours, ActiveHoursMaxSpan) +
			"Se o ajuste automático do horário ativo estiver ligado, o Windows pode mover a janela pelas suas horas de uso; nesse caso o item avisa que não está valendo e mostra onde escolher \"Manualmente\".",
		Evidence: tweaks.EvHigh, Sources: []tweaks.Source{srcActiveHours},
		Risk: tweaks.RiskSafe, Reboot: tweaks.RebootNone, Tags: []string{"estabilidade"}, OneClick: false,
		Recommend: neutral("escolha o horário em que o PC não pode reiniciar; a janela vale para todas as atualizações"),
		Ops: func(*facts.Facts) []tweaks.Op {
			return []tweaks.Op{
				tweaks.Reg(KeyActiveHours, "ActiveHoursStart", sys.DWord(uint32(start))),
				tweaks.Reg(KeyActiveHours, "ActiveHoursEnd", sys.DWord(uint32(end))),
			}
		},
		Applicable: func(f *facts.Facts) (bool, string) {
			if f.OS.ActiveHoursPolicy {
				return false, fmt.Sprintf("uma política de grupo fixa o horário ativo (das %02d:00 às %02d:00); o valor das Configurações é ignorado", f.OS.ActiveHoursPolicyStart, f.OS.ActiveHoursPolicyEnd)
			}
			return true, ""
		},
		Effect: func(f *facts.Facts) (bool, string) {
			if f != nil && f.OS.ActiveHoursAuto == 1 {
				return false, "o ajuste automático do horário ativo está ligado e o Windows pode mover a janela pelas suas horas de uso: em Configurações › Windows Update › Opções avançadas › Horário ativo, escolha \"Manualmente\""
			}
			return true, ""
		},
	}
}
