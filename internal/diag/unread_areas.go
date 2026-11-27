package diag

import "axion/internal/facts"

// unreadAreas maps each collection step and part (the names the collection
// puts in facts.Unread, and the parts of the event log read by the app) to
// the areas whose rules depend on it. A name not listed maps to no area.
// The literal names are those of internal/collect (the steps of Collect and
// its Part constants); a test checks that every one of them is listed.
var unreadAreas = map[string][]string{
	facts.StepOS:                               {AreaEstab, AreaSeguranca},
	"Processador":                              {AreaDesempenho, AreaEstab},
	"Placa de vídeo":                           {AreaDesempenho},
	"Memória RAM":                              {AreaDesempenho},
	"Placa-mãe e BIOS":                         {AreaDesempenho, AreaEstab},
	"Armazenamento":                            {AreaArmaz},
	"TRIM e otimização do SSD":                 {AreaArmaz},
	"Monitores":                                {AreaLatencia},
	"Rede":                                     {AreaRede},
	facts.StepSecurity:                         {AreaSeguranca},
	facts.StepGaming:                           {AreaDesempenho, AreaLatencia, AreaSeguranca},
	facts.StepPower:                            {AreaDesempenho},
	"Driver NVIDIA":                            {AreaDesempenho},
	"Perfil do driver NVIDIA":                  {AreaDesempenho, AreaLatencia},
	"Inicialização do sistema":                 {AreaDesempenho, AreaLatencia, AreaSeguranca},
	"Processos em execução":                    {AreaDesempenho},
	"Temperatura e limite térmico":             {AreaDesempenho},
	"Dispositivos com problema":                {AreaEstab},
	"Dispositivos PCI":                         {AreaEstab},
	"Arquivos temporários":                     {AreaArmaz},
	"Mouse, teclado e portas USB":              {AreaLatencia},
	facts.StepHealth:                           {AreaDesempenho, AreaEstab, AreaArmaz, AreaSeguranca},
	facts.StepVulnDrivers:                      {AreaSeguranca},
	facts.PartX3D:                              {AreaDesempenho},
	"Ajustes deixados por outros otimizadores": {AreaDesempenho, AreaEstab, AreaRede, AreaArmaz, AreaSeguranca},
	"Configurações de jogo › agendamento de GPU (driver de vídeo)":   {AreaDesempenho},
	"Armazenamento › saúde NVMe (driver do disco)":                   {AreaArmaz},
	"Mouse, teclado e portas USB › dispositivos USB e HID":           {AreaLatencia},
	"Perfil do driver NVIDIA › leitura pelo NVAPI (driver de vídeo)": {AreaDesempenho, AreaLatencia},
	PartWHEA:     {AreaEstab},
	PartSymptoms: {AreaEstab},
	PartBoots:    {AreaEstab},
	PartThermal:  {AreaEstab, AreaDesempenho},
}

// UnreadAreas returns the areas a part of the collection that was not read
// belongs to (none for an unknown name).
func UnreadAreas(name string) []string { return unreadAreas[name] }
