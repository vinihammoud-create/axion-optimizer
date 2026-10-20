package diag

import (
	"strconv"
	"strings"

	"axion/internal/facts"
	"axion/internal/tweaks"
)

var (
	srcLolWinRing0 = tweaks.Source{Title: "LOLDrivers — WinRing0.sys / WinRing0x64.sys (OpenLibSys): driver de acesso a hardware que expõe MSR, portas de E/S e memória física ao modo usuário", URL: "https://www.loldrivers.io/drivers/7bb5ff05-25f8-410d-ae99-c8e8f082d24f/"}
	srcMSWinRing0  = tweaks.Source{Title: "Microsoft Security Intelligence — VulnerableDriver:WinNT/Winring0.G: falta total de controle de acesso (CVE-2020-14979); lista programas antigos que o incluem; orienta atualizar ou desinstalar o programa", URL: "https://www.microsoft.com/en-us/wdsi/threats/malware-encyclopedia-description?Name=VulnerableDriver%3AWinNT%2FWinring0.G&ThreatID=2147947097"}
	srcLolAmiFl    = tweaks.Source{Title: "LOLDrivers — amifldrv64.sys: driver vulnerável (verificado), listado para obter privilégio de kernel", URL: "https://www.loldrivers.io/drivers/34fa6ba4-dc7c-4fd6-b947-8a0bb8ebd031/"}
	srcLolAmiGen   = tweaks.Source{Title: "LOLDrivers — amigendrv64.sys: driver vulnerável (verificado), carregado como serviço de kernel mesmo com a Integridade de Memória", URL: "https://www.loldrivers.io/drivers/5c45ae9e-cb6f-4eab-a070-b0187202e080/"}
	srcLolRwDrv    = tweaks.Source{Title: "LOLDrivers — rwdrv.sys (RWEverything): driver vulnerável, acesso a memória física, MSR e PCI", URL: "https://www.loldrivers.io/drivers/fbdd993b-47b1-4448-8c41-24c310802398/"}
)

var vulnDriverSources = map[string][]tweaks.Source{
	"winring0":  {srcMSWinRing0, srcLolWinRing0},
	"amifldrv":  {srcLolAmiFl},
	"amigendrv": {srcLolAmiGen},
	"rwdrv":     {srcLolRwDrv},
}

// vulnDriverFinding reports known vulnerable kernel drivers found on the PC.
// There is no automatic action: the driver belongs to a program, and the safe
// way out is to update or remove that program.
func vulnDriverFinding(found []facts.VulnDriver, add func(Finding)) {
	var lines []string
	var srcs []tweaks.Source
	seen := map[string]bool{}
	for _, d := range found {
		where := "registrado como serviço de driver"
		switch {
		case d.Loaded && d.Registered:
			where = "carregado no kernel agora e registrado como serviço de driver"
		case d.Loaded:
			where = "carregado no kernel agora"
		}
		line := d.Display + ": " + where
		if d.Registered && (d.State != "" || d.Start != "") {
			line += " (estado " + orText(d.State, "desconhecido") + ", início " + orText(d.Start, "desconhecido") + ")"
		}
		if d.Path != "" {
			line += ", arquivo " + d.Path
		}
		lines = append(lines, line+".")
		for _, s := range vulnDriverSources[d.ID] {
			if !seen[s.URL] {
				seen[s.URL] = true
				srcs = append(srcs, s)
			}
		}
	}
	origins := map[string]bool{}
	var from []string
	for _, d := range found {
		if !origins[d.Origin] {
			origins[d.Origin] = true
			from = append(from, d.Display+" costuma vir com "+d.Origin)
		}
	}
	title := "Driver de kernel vulnerável encontrado: " + found[0].Display
	if len(found) > 1 {
		title = "Drivers de kernel vulneráveis encontrados: " + strconv.Itoa(len(found))
	}
	ex := "Esses drivers dão a qualquer programa que rode como administrador acesso direto a memória física, portas de E/S ou registradores da CPU. " +
		"O LOLDrivers os cataloga como vulneráveis e, no caso do WinRing0, a Microsoft aponta a falta total de controle de acesso (CVE-2020-14979). " +
		"Encontrar o driver não prova que o PC foi atacado: ele costuma ficar para trás quando um programa de monitoramento, de placa ou de BIOS o instala. " +
		strings.Join(from, "; ") + ". O Axion não conhece todos os programas que trazem cada driver e só reconhece os nomes de arquivo e de serviço da lista dele (critério do Axion): " +
		"o mesmo driver pode vir com outro nome. " +
		"O que fazer: na página Programas, procure o aplicativo de origem e atualize para uma versão que não use o driver ou desinstale o que você não usa. " +
		"O Axion só lê e não carrega, não para nem apaga driver nenhum: apagar o arquivo à mão não adianta se o programa o recria ao abrir."
	add(Finding{ID: "sec.driver.vulneravel", Area: AreaSeguranca, Severity: SevWarning, Title: title,
		Evidence:    strings.Join(lines, " "),
		Explanation: ex,
		Actions:     []Action{{Kind: "page", Label: "Abrir Programas", Page: "programas"}},
		Sources:     srcs})
}
