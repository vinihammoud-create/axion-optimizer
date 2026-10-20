package apps

import (
	"strings"
	"testing"
)

// The catalog is consistent: unique ids, known categories, one way to
// install each item, verified ids well formed, the recommended item first
// in its category.
func TestCatalog(t *testing.T) {
	cats := map[string]bool{}
	for _, c := range Categories {
		cats[c.ID] = true
	}
	seen := map[string]bool{}
	firstOf := map[string]string{}
	for _, it := range All {
		if seen[it.ID] {
			t.Errorf("id repetido: %s", it.ID)
		}
		seen[it.ID] = true
		if !cats[it.Category] {
			t.Errorf("%s: categoria desconhecida %q", it.ID, it.Category)
		}
		ways := 0
		for _, w := range []string{it.Winget, it.Store, it.Manual} {
			if w != "" {
				ways++
			}
		}
		if ways != 1 {
			t.Errorf("%s: deve ter exatamente um caminho (winget, Store ou link): %+v", it.ID, it)
		}
		if it.Winget != "" && (!strings.Contains(it.Winget, ".") || strings.ContainsAny(it.Winget, " \t\"'")) {
			t.Errorf("%s: identificador winget inválido %q", it.ID, it.Winget)
		}
		if it.Manual != "" && !strings.HasPrefix(it.Manual, "https://") {
			t.Errorf("%s: link manual deve ser https: %q", it.ID, it.Manual)
		}
		if it.For == "" {
			t.Errorf("%s: sem \"para quem\"", it.ID)
		}
		if _, ok := firstOf[it.Category]; !ok {
			firstOf[it.Category] = it.ID
			if !it.Recommended {
				t.Errorf("%s: o primeiro item da categoria %s deve ser o recomendado", it.ID, it.Category)
			}
		} else if it.Recommended {
			t.Errorf("%s: só o primeiro item da categoria é o recomendado", it.ID)
		}
		for _, p := range append(append([]string{}, it.Profiles...), it.Suggested...) {
			if p != Competitive && p != Live && p != Work && p != Creation && p != Dev {
				t.Errorf("%s: perfil desconhecido %q", it.ID, p)
			}
		}
	}
	if len(All) < 60 {
		t.Errorf("catálogo curto: %d itens", len(All))
	}
}

// The preset follows the profile and the hardware.
func TestPreset(t *testing.T) {
	ghub, _ := Get("ghub")
	steam, _ := Get("steam")
	obs, _ := Get("obs")
	nvidia, _ := Get("nvidia")
	dell, _ := Get("dellupdate")
	hp, _ := Get("hpsa")
	anki, _ := Get("anki")
	ngen, _ := Get("ngenuity")
	razer, _ := Get("synapse")
	var none []Periph
	gaming := []Periph{{VID: 0x046D, Name: "G502 LIGHTSPEED Wireless Gaming Mouse"}}
	office := []Periph{{VID: 0x046D, Name: "USB Receiver"}, {VID: 0x046D, Name: "Logitech M185"}}
	if ghub.Preset(Competitive, none, "", "") != "" || ghub.Preset(Work, gaming, "", "") != "marcado" {
		t.Error("G HUB só com mouse Logitech G")
	}
	if ghub.Preset(Competitive, office, "", "") != "" {
		t.Error("um mouse de escritório da Logitech não marca o G HUB")
	}
	if !ghub.Matches(Periph{VID: 0x046D, PID: 0xC547, Name: "USB Receiver"}) || ghub.Matches(Periph{VID: 0x046D, PID: 0xC52B, Name: "USB Receiver"}) {
		t.Error("receptor LIGHTSPEED pelo PID; o Unifying não")
	}
	if !ghub.Matches(Periph{VID: 0x046D, Name: "PRO X Superlight"}) || !ghub.Matches(Periph{VID: 0x046D, Name: "Logitech G Pro Gaming Keyboard"}) || ghub.Matches(Periph{VID: 0x1532, Name: "Razer Viper"}) {
		t.Error("linha G da Logitech")
	}
	if !ngen.Matches(Periph{VID: 0x03F0, Name: "HyperX Pulsefire Haste 2"}) || ngen.Matches(Periph{VID: 0x03F0, Name: "HP 320M Mouse"}) {
		t.Error("HyperX pelo nome, não por todo mouse HP")
	}
	if !razer.Matches(Periph{VID: 0x1532, Name: "Razer DeathAdder V3"}) {
		t.Error("Razer por fabricante")
	}
	if steam.Preset(Competitive, none, "", "") != "marcado" || steam.Preset(Work, none, "", "") != "" {
		t.Error("Steam no competitivo, não no trabalho")
	}
	if obs.Preset(Live, none, "", "") != "marcado" || obs.Preset(Competitive, none, "", "") != "" {
		t.Error("OBS na live")
	}
	if nvidia.Preset(Work, none, "", "NVIDIA") != "marcado" || nvidia.Preset(Competitive, none, "", "AMD") != "" {
		t.Error("driver NVIDIA pela placa")
	}
	if dell.Preset(Competitive, none, "dell inc. dell inc.", "") != "marcado" || dell.Preset(Competitive, none, "", "") != "" {
		t.Error("Dell Update só em notebook Dell")
	}
	if !hp.LaptopMatch("HP HP") || !hp.LaptopMatch("Hewlett-Packard") || hp.LaptopMatch("ASUSTeK COMPUTER INC.") {
		t.Error("HP pelas duas grafias")
	}
	if anki.Preset(Work, none, "", "") != "sugerido" || anki.Preset(Competitive, none, "", "") != "" {
		t.Error("Anki sugerido no trabalho")
	}
	git, _ := Get("git")
	notion, _ := Get("notion")
	sevenzip, _ := Get("7zip")
	firefox, _ := Get("firefox")
	if !steam.NameMatch("Steam") || steam.NameMatch("SteamVR") || !ghub.NameMatch("Logitech G HUB 2024.9") || ghub.NameMatch("Logitech Options") {
		t.Error("nome instalado")
	}
	if !git.NameMatch("Git") || !git.NameMatch("Git version 2.47.1") || git.NameMatch("GitHub Desktop") || !notion.NameMatch("Notion 4.1.0") || notion.NameMatch("Notion Calendar") {
		t.Error("nome com versão ou outra palavra")
	}
	if !sevenzip.NameMatch("7-Zip 24.08 (x64)") || !firefox.NameMatch("Mozilla Firefox (x64 pt-BR)") {
		t.Error("edição entre parênteses")
	}
	o365, _ := Get("office")
	if o365.Preset(Work, none, "", "") != "sugerido" || !o365.Paid {
		t.Error("o pacote de empresas do Office só é sugerido")
	}
	if _, ok := Get("malwarebytes"); ok {
		t.Error("antivírus de segunda opinião fora do catálogo")
	}
	if sp, _ := Get("spotify"); sp.Winget != "" || sp.Store == "" {
		t.Error("Spotify pela Store (o instalador do site recusa elevação)")
	}
}

// winget's export JSON and upgrade table are read; exit codes classified.
func TestWingetParsing(t *testing.T) {
	m, err := ParseExport([]byte(`{"$schema":"x","Sources":[{"Packages":[{"PackageIdentifier":"Valve.Steam","Version":"1.0"},{"PackageIdentifier":"Discord.Discord"}],"SourceDetails":{"Name":"winget"}},{"Packages":[{"PackageIdentifier":"9NKSQGP7F2NH"}],"SourceDetails":{"Name":"msstore"}}]}`))
	if err != nil || len(m) != 3 || m["valve.steam"].Version != "1.0" || m["9nksqgp7f2nh"].ID != "9NKSQGP7F2NH" {
		t.Fatalf("export: %v %+v", err, m)
	}
	table := "Name                         Id                  Version      Available    Source\r\n" +
		"----------------------------------------------------------------------------------\r\n" +
		"Discord                      Discord.Discord     1.0.9212     1.0.9230     winget\r\n" +
		"Google Chrome (x64) comp…    Google.Chrome       130.0.1      131.0.6      winget\r\n" +
		"WhatsApp                     9NKSQGP7F2NH        Unknown      2.2450.1     msstore\r\n" +
		"3 upgrades available.\r\n"
	up := ParseUpgradeTable(table)
	if len(up) != 3 || up["discord.discord"].Available != "1.0.9230" || up["google.chrome"].Version != "130.0.1" || up["9nksqgp7f2nh"].Available != "2.2450.1" || up["9nksqgp7f2nh"].Version != "" {
		t.Errorf("tabela: %+v", up)
	}
	// pt-BR headers, a note whose last word is a source name, a cut id, an
	// unknown version and the "explicit targeting" table.
	ptbr := "Falha ao tentar atualizar a origem: msstore\r\n" +
		"Nome                                   ID                                     Versão        Disponível    Origem\r\n" +
		"-------------------------------------------------------------------------------------------------------------------\r\n" +
		"Moonlight                              MoonlightGameStreamingProject.Moonl…   6.0.0         6.1.0         winget\r\n" +
		"Microsoft Visual Studio Code           Microsoft.VisualStudioCode             1.94.0        1.95.0        winget\r\n" +
		"Microsoft Teams                        Microsoft.Teams                        Desconhecido  24295.605     winget\r\n" +
		"3 atualizações disponíveis.\r\n" +
		"\r\n" +
		"Os seguintes pacotes têm uma atualização disponível, mas exigem direcionamento explícito:\r\n" +
		"Nome                                   ID                                     Versão        Disponível    Origem\r\n" +
		"-------------------------------------------------------------------------------------------------------------------\r\n" +
		"Node.js LTS                            OpenJS.NodeJS.LTS                      < 22.0.0      22.11.0       winget\r\n" +
		"1 pacote(s) com atualização disponível.\r\n"
	up = ParseUpgradeTable(ptbr)
	if len(up) != 2 || up["microsoft.visualstudiocode"].Available != "1.95.0" || up["microsoft.teams"].Version != "" || up["microsoft.teams"].Available != "24295.605" {
		t.Errorf("tabela pt-BR: %+v", up)
	}
	if _, ok := up["atualizar"]; ok {
		t.Error("nota lida como pacote")
	}
	if m, err := ParseExport([]byte("\xef\xbb\xbf" + `{"Sources":[{"Packages":[{"PackageIdentifier":"Git.Git","Version":"Unknown"}]}]}`)); err != nil || m["git.git"].ID != "Git.Git" || m["git.git"].Version != "" {
		t.Errorf("export com BOM: %v %+v", err, m)
	}
	if o := Classify(0, ""); !o.OK || o.Skipped {
		t.Error("0")
	}
	if o := Classify(int(uint32(CodeAlreadyInstalled)), ""); !o.OK || !o.Skipped {
		t.Error("já instalado")
	}
	if o := Classify(-1978335135, ""); !o.OK || !o.Skipped { // the same code as a signed 32-bit value
		t.Error("código negativo")
	}
	if o := Classify(int(uint32(CodeRebootRequired)), ""); !o.OK || !o.Reboot {
		t.Error("reinício")
	}
	if o := Classify(int(uint32(CodeNoPackage)), ""); o.OK || !strings.Contains(o.Text, "não encontrado") {
		t.Error("não encontrado")
	}
	if o := Classify(1603, "Installer failed with exit code: 1603"); o.OK || !strings.Contains(o.Text, "1603") {
		t.Errorf("desconhecido: %+v", o)
	}
	if o := Classify(int(uint32(CodeShellExecute)), "Installer failed with exit code: 1603"); o.OK || !strings.Contains(o.Text, "terminou com erro") || !strings.Contains(o.Text, "1603") {
		t.Errorf("instalador com erro: %+v", o)
	}
	if o := Classify(int(uint32(CodeUpgradeAllFail)), ""); o.OK || !o.Partial {
		t.Errorf("upgrade --all parcial: %+v", o)
	}
	if o := Classify(-1978335146, ""); o.OK || !strings.Contains(o.Text, "administrador") { // 0x8A150056
		t.Errorf("elevação proibida: %+v", o)
	}
	if o := Classify(int(uint32(CodeDowngrade)), ""); !o.OK || !o.Skipped {
		t.Error("versão mais nova já instalada")
	}
	if LastLine("a\r\nb\r\n  - \r\n\r\n") != "b" {
		t.Error("última linha")
	}
	a := InstallArgs(Item{Winget: "Brave.Brave"})
	if strings.Join(a, " ") != "install --id Brave.Brave -e --silent --accept-package-agreements --accept-source-agreements --disable-interactivity" {
		t.Errorf("args: %v", a)
	}
	if s := strings.Join(InstallArgs(Item{Store: "9NKSQGP7F2NH"}), " "); !strings.Contains(s, "-s msstore") || strings.Contains(s, "--silent") {
		t.Errorf("args da Store: %s", s)
	}
}
