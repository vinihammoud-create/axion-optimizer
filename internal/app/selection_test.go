package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"axion/internal/sys/mocksys"
)

func scanned(t *testing.T) (*mocksys.Mock, *client) {
	t.Helper()
	m, c := setupServer(t)
	var j Job
	c.json("POST", "/api/scan", map[string]any{}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("análise falhou: %+v", j)
	}
	return m, c
}

func recommendedIDs(t *testing.T, c *client) []string {
	t.Helper()
	var plan Plan
	c.json("GET", "/api/plan?tier=recomendado", nil, &plan)
	var ids []string
	for _, it := range plan.Items {
		ids = append(ids, it.ID)
	}
	if len(ids) < 3 {
		t.Fatalf("plano recomendado pequeno demais: %v", ids)
	}
	return ids
}

func applyAll(t *testing.T, c *client, ids []string) {
	t.Helper()
	var j Job
	c.json("POST", "/api/apply", ApplyRequest{IDs: ids}, &j)
	if j = c.wait(j.ID); j.Status != JobDone {
		t.Fatalf("aplicação falhou: %+v", j)
	}
}

func selectionFileJSON(ids ...string) string {
	b, _ := json.Marshal(SelectionFile{Format: SelectionFormat, FormatVersion: SelectionFormatVersion, AxionVersion: "test", IDs: ids})
	return string(b)
}

// The exported file has the IDs applied by the Axion and the metadata, and
// nothing read from the PC.
func TestSelectionExportHasOnlyIDsAndMetadata(t *testing.T) {
	_, c := scanned(t)
	if code, _ := c.do("GET", "/api/selection/export", nil, nil); code != 400 {
		t.Errorf("exportar sem nada aplicado deveria ser recusado: %d", code)
	}
	ids := recommendedIDs(t, c)
	applied := ids[:3]
	applyAll(t, c, applied)

	code, b := c.do("GET", "/api/selection/export", nil, nil)
	if code != 200 {
		t.Fatalf("exportar: %d %s", code, b)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"axionVersion", "created", "format", "formatVersion", "ids"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("campos do arquivo: %v, esperado %v", keys, want)
	}
	var sf SelectionFile
	_ = json.Unmarshal(b, &sf)
	want := append([]string(nil), applied...)
	sort.Strings(want)
	if sf.Format != SelectionFormat || sf.FormatVersion != SelectionFormatVersion || sf.AxionVersion != "test" || sf.Created.IsZero() || !reflect.DeepEqual(sf.IDs, want) {
		t.Errorf("arquivo exportado: %+v, IDs esperados %v", sf, want)
	}
	host, users := c.s.App.anonNames()
	for _, s := range append(users, host, c.s.App.P.DataDir, `C:\`, `\\`) {
		if s != "" && strings.Contains(strings.ToLower(string(b)), strings.ToLower(s)) {
			t.Errorf("o arquivo traz dado do PC (%q): %s", s, b)
		}
	}
	// What was undone leaves the file.
	var j Job
	c.json("POST", "/api/revert", RevertRequest{IDs: applied[:1]}, &j)
	c.wait(j.ID)
	c.json("GET", "/api/selection/export", nil, &sf)
	if len(sf.IDs) != 2 {
		t.Errorf("depois de desfazer um item, o arquivo deveria ter 2: %v", sf.IDs)
	}
}

// A file out of the format is refused with a reason, before anything else.
func TestSelectionImportRejectsMalformed(t *testing.T) {
	_, c := scanned(t)
	many := make([]string, maxSelectionIDs+1)
	for i := range many {
		many[i] = fmt.Sprintf("x%d", i)
	}
	cases := map[string]string{
		"vazio":              "",
		"não é JSON":         "isto não é json",
		"outro formato":      `{"format":"outro","formatVersion":1,"ids":["a"]}`,
		"versão nova":        `{"format":"axion-selecao","formatVersion":2,"ids":["a"]}`,
		"sem versão":         `{"format":"axion-selecao","ids":["a"]}`,
		"campo desconhecido": `{"format":"axion-selecao","formatVersion":1,"ids":["a"],"pc":"MEU-PC"}`,
		"conteúdo depois":    selectionFileJSON("a") + `{"x":1}`,
		"lista vazia":        selectionFileJSON(),
		"ID com espaço":      selectionFileJSON("a b"),
		"ID vazio":           selectionFileJSON(""),
		"ID com controle":    selectionFileJSON("a\x00"),
		"ID longo demais":    selectionFileJSON(strings.Repeat("a", maxSelectionIDLen+1)),
		"IDs demais":         selectionFileJSON(many...),
		"grande demais":      selectionFileJSON("a") + strings.Repeat(" ", maxSelectionBytes),
		"IDs não são texto":  `{"format":"axion-selecao","formatVersion":1,"ids":[1,2]}`,
	}
	for name, content := range cases {
		code, b := c.do("POST", "/api/selection/import", SelectionImportRequest{Content: content}, nil)
		if code != 400 || !strings.Contains(string(b), "arquivo de seleção inválido") {
			t.Errorf("%s: deveria ser recusado com o motivo: %d %s", name, code, b)
		}
	}
	if code, b := c.do("POST", "/api/selection/import", SelectionImportRequest{Content: selectionFileJSON("a")}, nil); code != 200 {
		t.Errorf("arquivo válido recusado: %d %s", code, b)
	}
}

// Importing lists what is unknown or does not enter the plan, and changes
// nothing: no journal entry, no value, no job.
func TestSelectionImportReportsSkippedAndAppliesNothing(t *testing.T) {
	m, c := scanned(t)
	ids := recommendedIDs(t, c)
	applyAll(t, c, ids[:1]) // already on

	views, err := c.s.App.TweakViews(false)
	if err != nil {
		t.Fatal(err)
	}
	na, risky := "", ""
	for _, v := range views {
		if na == "" && !v.State.Applicable {
			na = v.ID
		}
		if risky == "" && v.State.Applicable && v.State.Status != "on" && v.RiskLevel > 1 && v.State.Rec.Level != 3 {
			risky = v.ID
		}
	}
	if na == "" || risky == "" {
		t.Fatalf("o simulador deveria ter um item que não se aplica (%q) e um com risco (%q)", na, risky)
	}
	file := selectionFileJSON(append([]string{"nao.existe", "startup:hkcu-run:Algo", ids[0], na, risky}, ids[1:]...)...)

	before, journal := m.World(), len(c.s.App.Engine.J.Snapshot())
	var res SelectionImport
	c.json("POST", "/api/selection/import", SelectionImportRequest{Content: file}, &res)

	if !reflect.DeepEqual(res.Unknown, []string{"nao.existe"}) {
		t.Errorf("desconhecidos: %v", res.Unknown)
	}
	why := map[string]string{}
	for _, s := range res.Skipped {
		why[s.ID] = s.Reason
	}
	for id, part := range map[string]string{"startup:hkcu-run:Algo": "Inicialização", ids[0]: "já está ativa", na: "não se aplica", risky: "risco"} {
		if !strings.Contains(why[id], part) {
			t.Errorf("%s deveria ficar de fora com %q: %q", id, part, why[id])
		}
	}
	var got []string
	for _, it := range res.Plan.Items {
		if !it.Checked {
			t.Errorf("item do arquivo deveria vir marcado: %s", it.ID)
		}
		got = append(got, it.ID)
	}
	sort.Strings(got)
	want := append([]string(nil), ids[1:]...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("itens do plano: %v, esperado %v", got, want)
	}
	if res.Plan.Title != "Seleção importada" || res.AxionVersion != "test" {
		t.Errorf("plano: %q, versão do arquivo %q", res.Plan.Title, res.AxionVersion)
	}

	if n := len(c.s.App.Engine.J.Snapshot()); n != journal {
		t.Errorf("importar mudou o histórico: %d -> %d entradas", journal, n)
	}
	if d := mocksys.DiffWorld(before, m.World()); len(d) > 0 {
		t.Errorf("importar mudou o PC:\n  %s", strings.Join(d, "\n  "))
	}
	if _, running := c.s.App.Jobs.Running(); running {
		t.Error("importar iniciou uma tarefa")
	}
}

// The file of one PC opens the plan on another, with the same items.
func TestSelectionExportImportOtherPC(t *testing.T) {
	_, a := scanned(t)
	ids := recommendedIDs(t, a)
	applyAll(t, a, ids)
	_, file := a.do("GET", "/api/selection/export", nil, nil)

	_, b := scanned(t)
	var res SelectionImport
	b.json("POST", "/api/selection/import", SelectionImportRequest{Content: string(file)}, &res)
	if len(res.Unknown) > 0 || len(res.Skipped) > 0 || len(res.Plan.Items) != len(ids) {
		t.Errorf("no outro PC: %d itens de %d, desconhecidos %v, de fora %+v", len(res.Plan.Items), len(ids), res.Unknown, res.Skipped)
	}
}
