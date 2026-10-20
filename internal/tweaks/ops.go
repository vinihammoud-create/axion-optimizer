package tweaks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"axion/internal/facts"
	"axion/internal/sys"
)

// Ctx is passed to every operation.
type Ctx struct {
	Context context.Context
	P       *sys.Platform
	F       *facts.Facts
	Log     func(format string, args ...any)
	// Touched holds the targets (Op.Target) of the operations an apply or a
	// revert changed, for Post.
	Touched []string
}

// TouchedTarget reports whether an operation on target changed in this
// apply or revert.
func (c *Ctx) TouchedTarget(target string) bool {
	for _, t := range c.Touched {
		if t == target {
			return true
		}
	}
	return false
}

func (c *Ctx) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(format, args...)
	}
}

// Op is one atomic, reversible change. Every op can capture the current state
// (Capture), tell whether the system is already in the desired state
// (InTarget), apply the desired state and restore a captured snapshot.
type Op interface {
	Kind() string
	// Target identifies the resource touched (used for conflict tracking).
	Target() string
	// Describe is the exact technical change, shown to the user.
	Describe() string
	Capture(c *Ctx) (json.RawMessage, error)
	InTarget(c *Ctx) (bool, error)
	Apply(c *Ctx) error
	Restore(c *Ctx, snap json.RawMessage) error
	// Current renders the current state for the technical details panel.
	Current(c *Ctx) string
}

// ---------------------------------------------------------------------------
// Registry value
// ---------------------------------------------------------------------------

// RegOp sets (or deletes, when Want is absent) one registry value.
type RegOp struct {
	Path string
	Name string
	Want sys.RegValue
	// AltOK lists other values that count as "already in target" (e.g. both
	// 0 and absent mean off). Apply still writes Want.
	AltOK []sys.RegValue
	// Match, when set, decides InTarget instead of Want/AltOK: a value whose
	// bytes change on every write (StartupApproved carries the FILETIME of
	// the change) is in target by meaning, not by bytes. Apply still writes
	// Want; Capture, Restore and the snapshot comparison stay byte-exact.
	Match func(v sys.RegValue) bool
}

type regSnap struct {
	Value      sys.RegValue `json:"value"`
	KeyExisted bool         `json:"keyExisted"`
	// Ancestor is the deepest key of Path that existed before Axion wrote the
	// value; keys below it were created by Axion and are pruned on restore.
	Ancestor string `json:"ancestor,omitempty"`
}

func parentKey(p string) string {
	i := strings.LastIndex(p, `\`)
	if i < 0 {
		return ""
	}
	return p[:i]
}

func deepestExisting(r sys.Registry, p string) string {
	for cur := p; cur != ""; cur = parentKey(cur) {
		if !strings.Contains(cur, `\`) {
			return cur // hive root always exists
		}
		if ok, err := r.KeyExists(cur); err == nil && ok {
			return cur
		}
	}
	return ""
}

// RegMatch builds a RegOp whose target state is decided by match.
func RegMatch(path, name string, want sys.RegValue, match func(sys.RegValue) bool) *RegOp {
	op := Reg(path, name, want)
	op.Match = match
	return op
}

// Reg builds a RegOp.
func Reg(path, name string, want sys.RegValue, alt ...sys.RegValue) *RegOp {
	return &RegOp{Path: sys.NormalizeRegPath(path), Name: name, Want: want, AltOK: alt}
}

func (o *RegOp) Kind() string { return "registry" }
func (o *RegOp) Target() string {
	return "reg:" + strings.ToLower(o.Path) + "|" + strings.ToLower(o.Name)
}
func (o *RegOp) Describe() string {
	if !o.Want.Exists {
		return fmt.Sprintf(`%s → remover valor "%s"`, o.Path, o.Name)
	}
	return fmt.Sprintf(`%s → "%s" = %s [%s]`, o.Path, o.Name, o.Want.Display(), o.Want.Kind)
}

func (o *RegOp) Capture(c *Ctx) (json.RawMessage, error) {
	v, err := c.P.Reg.GetValue(o.Path, o.Name)
	if err != nil {
		return nil, err
	}
	ke, err := c.P.Reg.KeyExists(o.Path)
	if err != nil {
		return nil, err
	}
	s := regSnap{Value: v, KeyExisted: ke}
	if !ke {
		s.Ancestor = deepestExisting(c.P.Reg, o.Path)
	}
	return json.Marshal(s)
}

func (o *RegOp) InTarget(c *Ctx) (bool, error) {
	v, err := c.P.Reg.GetValue(o.Path, o.Name)
	if err != nil {
		return false, err
	}
	if o.Match != nil {
		return o.Match(v), nil
	}
	if v.Equal(o.Want) {
		return true, nil
	}
	for _, a := range o.AltOK {
		if v.Equal(a) {
			return true, nil
		}
	}
	return false, nil
}

func (o *RegOp) Apply(c *Ctx) error {
	if !o.Want.Exists {
		return c.P.Reg.DeleteValue(o.Path, o.Name)
	}
	return c.P.Reg.SetValue(o.Path, o.Name, o.Want)
}

func (o *RegOp) Restore(c *Ctx, raw json.RawMessage) error {
	var s regSnap
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s.Value.Exists {
		return c.P.Reg.SetValue(o.Path, o.Name, s.Value)
	}
	if err := c.P.Reg.DeleteValue(o.Path, o.Name); err != nil {
		return err
	}
	if !s.KeyExisted && s.Ancestor != "" && len(s.Ancestor) < len(o.Path) {
		// Remove keys Axion created, leaf first, stopping at the first key
		// that still has content (a sibling value/key) or at the ancestor.
		for p := o.Path; len(p) > len(s.Ancestor) && strings.Contains(p, `\`); p = parentKey(p) {
			deleted, err := c.P.Reg.DeleteKeyIfEmpty(p)
			if err != nil || !deleted {
				break
			}
		}
	}
	return nil
}

func (o *RegOp) Current(c *Ctx) string {
	v, err := c.P.Reg.GetValue(o.Path, o.Name)
	if err != nil {
		return "erro: " + err.Error()
	}
	return v.Display()
}

// SnapshotValue decodes the "before" value from a RegOp snapshot (for reports).
func SnapshotValue(raw json.RawMessage) (sys.RegValue, bool) {
	var s regSnap
	if json.Unmarshal(raw, &s) != nil {
		return sys.RegValue{}, false
	}
	return s.Value, true
}

// ---------------------------------------------------------------------------
// Service start type
// ---------------------------------------------------------------------------

// SvcOp sets a service start type and optionally stops it.
type SvcOp struct {
	Name    string
	Want    sys.ServiceStart
	StopNow bool // stop the service when applying Disabled/Manual
}

type svcSnap struct {
	Exists  bool             `json:"exists"`
	Start   sys.ServiceStart `json:"start"`
	Running bool             `json:"running"`
}

// Svc builds a SvcOp.
func Svc(name string, want sys.ServiceStart, stop bool) *SvcOp {
	return &SvcOp{Name: name, Want: want, StopNow: stop}
}

func (o *SvcOp) Kind() string   { return "service" }
func (o *SvcOp) Target() string { return "svc:" + strings.ToLower(o.Name) }
func (o *SvcOp) Describe() string {
	s := fmt.Sprintf("Serviço %s → inicialização %s", o.Name, o.Want)
	if o.StopNow {
		s += " (e parar agora)"
	}
	return s
}

func (o *SvcOp) Capture(c *Ctx) (json.RawMessage, error) {
	i, err := c.P.Svc.Query(o.Name)
	if err != nil {
		return nil, err
	}
	// A service that is there but whose start type could not be read (a
	// protected service) has no value to go back to: refuse instead of
	// recording "unknown", which the undo could not write.
	if i.Exists && i.Start == sys.StartUnknown {
		return nil, fmt.Errorf("o Windows não deixou ler o tipo de inicialização do serviço %s", o.Name)
	}
	return json.Marshal(svcSnap{Exists: i.Exists, Start: i.Start, Running: i.Running})
}

func (o *SvcOp) InTarget(c *Ctx) (bool, error) {
	i, err := c.P.Svc.Query(o.Name)
	if err != nil {
		return false, err
	}
	if !i.Exists {
		return true, nil // nothing to do on this system
	}
	return i.Start == o.Want, nil
}

func (o *SvcOp) Apply(c *Ctx) error {
	i, err := c.P.Svc.Query(o.Name)
	if err != nil {
		return err
	}
	if !i.Exists {
		return nil
	}
	if err := c.P.Svc.SetStart(o.Name, o.Want); err != nil {
		return err
	}
	if o.StopNow && i.Running && (o.Want == sys.StartDisabled || o.Want == sys.StartManual) {
		if err := c.P.Svc.Stop(c.Context, o.Name); err != nil {
			c.logf("aviso: não foi possível parar %s agora: %v (a mudança vale após reiniciar)", o.Name, err)
		}
	}
	return nil
}

func (o *SvcOp) Restore(c *Ctx, raw json.RawMessage) error {
	var s svcSnap
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if !s.Exists {
		return nil
	}
	if err := c.P.Svc.SetStart(o.Name, s.Start); err != nil {
		return err
	}
	if s.Running {
		if i, err := c.P.Svc.Query(o.Name); err == nil && !i.Running {
			if err := c.P.Svc.Start(o.Name); err != nil {
				c.logf("aviso: %s restaurado, mas não iniciou agora: %v", o.Name, err)
			}
		}
	}
	return nil
}

func (o *SvcOp) Current(c *Ctx) string {
	i, err := c.P.Svc.Query(o.Name)
	if err != nil {
		return "erro: " + err.Error()
	}
	if !i.Exists {
		return "(serviço não existe neste Windows)"
	}
	st := "parado"
	if i.Running {
		st = "em execução"
	}
	return fmt.Sprintf("%s, %s", i.Start, st)
}

// ---------------------------------------------------------------------------
// Power setting (active scheme, AC and/or DC index)
// ---------------------------------------------------------------------------

// Power subgroup/setting GUIDs (documented in Microsoft Learn power settings reference).
const (
	SubProcessor   = "54533251-82be-4824-96c1-47b60b740d00"
	SetCoreParkMin = "0cc5b647-c1df-4637-891a-dec35c318583" // CPMINCORES (%)
	SetPerfBoost   = "be337238-0d82-4146-a960-4f3749d470c7" // PERFBOOSTMODE
	SetProcMin     = "893dee8e-2bef-41e0-89c6-b55d0929964c" // PROCTHROTTLEMIN (%)
	SetProcMax     = "bc5038f7-23e0-4960-96da-33abaf5935ec" // PROCTHROTTLEMAX (%)
	SubUSB         = "2a737441-1930-4402-8d77-b2bebba308a3"
	SetUSBSuspend  = "48e6b7a6-50f5-4782-a5d4-53bb8f07e226" // 0 = disabled, 1 = enabled
	SubPCIe        = "501a4d13-42af-4429-9fd1-a8218c268e20"
	SetASPM        = "ee12f906-d277-404b-b6da-e5fa1a576df5" // 0 off, 1 moderate, 2 maximum
	SubDisk        = "0012ee47-9041-4b5d-9b77-535fba8b1442"
	SetDiskIdle    = "6738e2c4-e8a5-4a42-b16a-e040e769756e" // turn off hard disk after (s)
)

// PowerOp writes an AC and/or DC value on the active scheme.
type PowerOp struct {
	Sub, Setting, Label string
	AC, DC              *uint32 // nil = do not touch
}

type powerSnap struct {
	Scheme string  `json:"scheme"`
	AC     *uint32 `json:"ac,omitempty"`
	DC     *uint32 `json:"dc,omitempty"`
}

// U32 returns a pointer to v.
func U32(v uint32) *uint32 { return &v }

// PowerSetting builds a PowerOp.
func PowerSetting(label, sub, setting string, ac, dc *uint32) *PowerOp {
	return &PowerOp{Sub: sub, Setting: setting, Label: label, AC: ac, DC: dc}
}

func (o *PowerOp) Kind() string   { return "power" }
func (o *PowerOp) Target() string { return "power:" + o.Sub + "/" + o.Setting }
func (o *PowerOp) Describe() string {
	var parts []string
	if o.AC != nil {
		parts = append(parts, fmt.Sprintf("na tomada = %d", *o.AC))
	}
	if o.DC != nil {
		parts = append(parts, fmt.Sprintf("na bateria = %d", *o.DC))
	}
	return fmt.Sprintf("Plano de energia ativo → %s (%s\\%s): %s", o.Label, o.Sub[:8], o.Setting[:8], strings.Join(parts, ", "))
}

func (o *PowerOp) Capture(c *Ctx) (json.RawMessage, error) {
	sch, err := c.P.Power.ActiveScheme()
	if err != nil {
		return nil, err
	}
	s := powerSnap{Scheme: sch}
	if o.AC != nil {
		v, err := c.P.Power.ReadValue(sch, o.Sub, o.Setting, true)
		if err != nil {
			return nil, err
		}
		s.AC = &v
	}
	if o.DC != nil {
		v, err := c.P.Power.ReadValue(sch, o.Sub, o.Setting, false)
		if err != nil {
			return nil, err
		}
		s.DC = &v
	}
	return json.Marshal(s)
}

func (o *PowerOp) InTarget(c *Ctx) (bool, error) {
	sch, err := c.P.Power.ActiveScheme()
	if err != nil {
		return false, err
	}
	if o.AC != nil {
		v, err := c.P.Power.ReadValue(sch, o.Sub, o.Setting, true)
		if err != nil || v != *o.AC {
			return false, err
		}
	}
	if o.DC != nil {
		v, err := c.P.Power.ReadValue(sch, o.Sub, o.Setting, false)
		if err != nil || v != *o.DC {
			return false, err
		}
	}
	return true, nil
}

func (o *PowerOp) Apply(c *Ctx) error {
	sch, err := c.P.Power.ActiveScheme()
	if err != nil {
		return err
	}
	if o.AC != nil {
		if err := c.P.Power.WriteValue(sch, o.Sub, o.Setting, true, *o.AC); err != nil {
			return err
		}
	}
	if o.DC != nil {
		if err := c.P.Power.WriteValue(sch, o.Sub, o.Setting, false, *o.DC); err != nil {
			return err
		}
	}
	return c.P.Power.SetActiveScheme(sch) // re-activate so the change takes effect now
}

func (o *PowerOp) Restore(c *Ctx, raw json.RawMessage) error {
	var s powerSnap
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s.Scheme != "" && !schemeExists(c, s.Scheme) {
		// The plan the value was written into was deleted since: what Axion
		// changed went with it, so there is nothing left to restore.
		c.logf("plano de energia %s não existe mais: nada a restaurar em %s", s.Scheme, o.Label)
		return nil
	}
	if s.AC != nil {
		if err := c.P.Power.WriteValue(s.Scheme, o.Sub, o.Setting, true, *s.AC); err != nil {
			return err
		}
	}
	if s.DC != nil {
		if err := c.P.Power.WriteValue(s.Scheme, o.Sub, o.Setting, false, *s.DC); err != nil {
			return err
		}
	}
	if act, err := c.P.Power.ActiveScheme(); err == nil && strings.EqualFold(act, s.Scheme) {
		return c.P.Power.SetActiveScheme(act)
	}
	return nil
}

func (o *PowerOp) Current(c *Ctx) string {
	sch, err := c.P.Power.ActiveScheme()
	if err != nil {
		return "erro: " + err.Error()
	}
	var parts []string
	if v, err := c.P.Power.ReadValue(sch, o.Sub, o.Setting, true); err == nil {
		parts = append(parts, fmt.Sprintf("tomada=%d", v))
	}
	if v, err := c.P.Power.ReadValue(sch, o.Sub, o.Setting, false); err == nil {
		parts = append(parts, fmt.Sprintf("bateria=%d", v))
	}
	if len(parts) == 0 {
		return "(não disponível neste plano)"
	}
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Active power scheme
// ---------------------------------------------------------------------------

// SchemeOp activates a power scheme. When Want is the Ultimate Performance
// template GUID, the template is duplicated first (Windows does not expose it
// by default on most editions) and the duplicate is deleted on restore.
type SchemeOp struct {
	Want  string
	Label string
}

type schemeSnap struct {
	Previous string `json:"previous"`
	Created  string `json:"created,omitempty"` // unused; kept for old journals
}

// schemeExists reports whether a power plan is still installed. When the
// list cannot be read the plan is assumed to exist (the caller then fails
// with the real error instead of skipping a restore).
func schemeExists(c *Ctx, guid string) bool {
	list, err := c.P.Power.Schemes()
	if err != nil {
		return true
	}
	for _, sc := range list {
		if strings.EqualFold(sc.GUID, guid) {
			return true
		}
	}
	return false
}

// Scheme builds a SchemeOp.
func Scheme(guid, label string) *SchemeOp {
	return &SchemeOp{Want: strings.ToLower(guid), Label: label}
}

func (o *SchemeOp) Kind() string   { return "powerscheme" }
func (o *SchemeOp) Target() string { return "power:active-scheme" }
func (o *SchemeOp) Describe() string {
	if o.Want == "" {
		return "Plano de energia ativo"
	}
	return fmt.Sprintf("Ativar plano de energia \"%s\" (%s)", o.Label, o.Want)
}

// ultimateMarker is written into the scheme description registry so Axion can
// recognise the duplicate it created on a previous run.
const ultimateRegPath = `HKLM\SOFTWARE\Axion\Power`

func (o *SchemeOp) resolve(c *Ctx) (string, error) {
	if o.Want != sys.SchemeUltimateTemplate {
		return o.Want, nil
	}
	// Reuse a duplicate previously created by Axion if it still exists.
	v, err := c.P.Reg.GetValue(ultimateRegPath, "UltimateGuid")
	if err == nil && v.Exists && v.Str != "" {
		if list, err := c.P.Power.Schemes(); err == nil {
			for _, s := range list {
				if strings.EqualFold(s.GUID, v.Str) {
					return s.GUID, nil
				}
			}
		}
	}
	// A scheme with the template GUID itself may exist (Workstation editions).
	list, err := c.P.Power.Schemes()
	if err != nil {
		return "", nil
	}
	for _, s := range list {
		if strings.EqualFold(s.GUID, sys.SchemeUltimateTemplate) {
			return s.GUID, nil
		}
	}
	// A copy created outside Axion (powercfg -duplicatescheme, another
	// optimizer) keeps the template's name: use it instead of adding one more
	// copy to the list. It is not Axion's, so the undo never deletes it.
	if act, err := c.P.Power.ActiveScheme(); err == nil {
		for _, s := range list {
			if strings.EqualFold(s.GUID, act) && UltimateCopyName(s.Name) {
				return s.GUID, nil
			}
		}
	}
	for _, s := range list {
		if UltimateCopyName(s.Name) {
			return s.GUID, nil
		}
	}
	return "", nil
}

// ultimateNames are the names Windows gives the Ultimate Performance plan in
// the languages Axion's users are most likely to have.
var ultimateNames = []string{"desempenho máximo", "ultimate performance", "máximo rendimiento", "rendimiento máximo", "ultimative leistung", "performances optimales", "prestazioni eccellenti"}

// UltimateCopyName reports whether a power plan name is the Ultimate
// Performance plan's.
func UltimateCopyName(name string) bool {
	n := strings.ToLower(strings.Join(strings.Fields(name), " "))
	for _, u := range ultimateNames {
		if n == u {
			return true
		}
	}
	return false
}

func (o *SchemeOp) Capture(c *Ctx) (json.RawMessage, error) {
	act, err := c.P.Power.ActiveScheme()
	if err != nil {
		return nil, err
	}
	return json.Marshal(schemeSnap{Previous: act})
}

func (o *SchemeOp) InTarget(c *Ctx) (bool, error) {
	act, err := c.P.Power.ActiveScheme()
	if err != nil {
		return false, err
	}
	want, err := o.resolve(c)
	if err != nil || want == "" {
		return false, err
	}
	return strings.EqualFold(act, want), nil
}

// Apply activates the plan (creating the Ultimate Performance copy first when
// needed; its GUID is recorded so Restore can delete it).
func (o *SchemeOp) Apply(c *Ctx) error {
	want, err := o.resolve(c)
	if err != nil {
		return err
	}
	if want == "" {
		g, err := c.P.Power.DuplicateScheme(sys.SchemeUltimateTemplate)
		if err != nil {
			return fmt.Errorf("não foi possível criar o plano Desempenho Máximo: %w", err)
		}
		// Without the marker the undo could not recognise (and delete) the
		// copy: give up instead of leaving an orphan plan behind.
		if err := c.P.Reg.SetValue(ultimateRegPath, "UltimateGuid", sys.SZ(g)); err != nil {
			_ = c.P.Power.DeleteScheme(g)
			return fmt.Errorf("não foi possível registrar o plano Desempenho Máximo criado: %w", err)
		}
		want = g
		if err := c.P.Power.SetActiveScheme(want); err != nil {
			o.removeDuplicate(c, "")
			return err
		}
		return nil
	}
	return c.P.Power.SetActiveScheme(want)
}

func (o *SchemeOp) Restore(c *Ctx, raw json.RawMessage) error {
	var s schemeSnap
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s.Previous == "" {
		return nil
	}
	prev := s.Previous
	if !schemeExists(c, prev) {
		// The previous plan was deleted since: Balanced, the Windows default,
		// is the closest state that still exists.
		c.logf("o plano de energia anterior (%s) não existe mais: ativando o Equilibrado", prev)
		prev = sys.SchemeBalanced
	}
	if err := c.P.Power.SetActiveScheme(prev); err != nil {
		return err
	}
	// An op rebuilt from an old journal has no Want; removeDuplicate only
	// deletes the copy Axion itself recorded, so it is safe to run then too.
	if o.Want == sys.SchemeUltimateTemplate || o.Want == "" {
		o.removeDuplicate(c, prev)
	}
	return nil
}

// removeDuplicate deletes the copy of the Ultimate Performance template that
// Axion created (recognised by the GUID Axion recorded when creating it),
// unless it is the plan being restored or the active one. A failure only
// leaves an extra plan in the list, so it is logged, not returned.
func (o *SchemeOp) removeDuplicate(c *Ctx, keep string) {
	v, err := c.P.Reg.GetValue(ultimateRegPath, "UltimateGuid")
	if err != nil || !v.Exists || v.Str == "" {
		return
	}
	g := v.Str
	if strings.EqualFold(g, keep) || strings.EqualFold(g, sys.SchemeUltimateTemplate) {
		return
	}
	if act, err := c.P.Power.ActiveScheme(); err != nil || strings.EqualFold(act, g) {
		return
	}
	exists := false
	if list, err := c.P.Power.Schemes(); err == nil {
		for _, sc := range list {
			if strings.EqualFold(sc.GUID, g) {
				exists = true
			}
		}
	}
	if exists {
		if err := c.P.Power.DeleteScheme(g); err != nil {
			c.logf("aviso: não foi possível apagar a cópia do plano Desempenho Máximo (%s): %v", g, err)
			return
		}
	}
	_ = c.P.Reg.DeleteValue(ultimateRegPath, "UltimateGuid")
	if ok, err := c.P.Reg.DeleteKeyIfEmpty(ultimateRegPath); err == nil && ok {
		_, _ = c.P.Reg.DeleteKeyIfEmpty(parentKey(ultimateRegPath))
	}
}

func (o *SchemeOp) Current(c *Ctx) string {
	act, err := c.P.Power.ActiveScheme()
	if err != nil {
		return "erro: " + err.Error()
	}
	name := act
	if list, err := c.P.Power.Schemes(); err == nil {
		for _, s := range list {
			if strings.EqualFold(s.GUID, act) {
				name = s.Name + " (" + act + ")"
			}
		}
	}
	return name
}

// ---------------------------------------------------------------------------
// Custom operation (Go functions + JSON snapshot)
// ---------------------------------------------------------------------------

// FuncOp is an escape hatch for changes that are not a single value
// (per-interface registry values, BCD elements, scheduled tasks, QoS policies…).
// Snapshot must capture everything needed to restore exactly.
type FuncOp struct {
	KindName string
	ID       string
	Desc     string
	// Spec holds the constructor parameters, recorded in the journal so the op
	// can be rebuilt by the builder registered for KindName (RegisterKind)
	// even when the facts that produced it have changed.
	Spec    any
	SnapFn  func(c *Ctx) (any, error)
	InFn    func(c *Ctx) (bool, error)
	ApplyFn func(c *Ctx) error
	// RestoreFn receives the raw snapshot produced by SnapFn.
	RestoreFn func(c *Ctx, raw json.RawMessage) error
	CurFn     func(c *Ctx) string
	// EqFn optionally decides whether two snapshots describe the same state
	// (default: identical JSON), e.g. to ignore parts Windows may remove.
	EqFn func(a, b json.RawMessage) bool
}

// SnapEqual implements the engine's snapshot comparison.
func (o *FuncOp) SnapEqual(a, b json.RawMessage) bool {
	if o.EqFn != nil {
		return o.EqFn(a, b)
	}
	return canonicalEqual(a, b)
}

// SpecJSON returns the serialized constructor parameters (nil if none).
func (o *FuncOp) SpecJSON() json.RawMessage {
	if o.Spec == nil {
		return nil
	}
	b, err := json.Marshal(o.Spec)
	if err != nil {
		return nil
	}
	return b
}

var kindBuilders = map[string]func(spec json.RawMessage) (Op, error){}

// RegisterKind registers a builder that recreates FuncOps of a kind from the
// Spec stored in the journal. Called from init() by packages defining ops.
func RegisterKind(kind string, build func(spec json.RawMessage) (Op, error)) {
	kindBuilders[kind] = build
}

func specOf(op Op) json.RawMessage {
	if s, ok := op.(interface{ SpecJSON() json.RawMessage }); ok {
		return s.SpecJSON()
	}
	return nil
}

func (o *FuncOp) Kind() string     { return o.KindName }
func (o *FuncOp) Target() string   { return o.KindName + ":" + o.ID }
func (o *FuncOp) Describe() string { return o.Desc }
func (o *FuncOp) Capture(c *Ctx) (json.RawMessage, error) {
	v, err := o.SnapFn(c)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
func (o *FuncOp) InTarget(c *Ctx) (bool, error)             { return o.InFn(c) }
func (o *FuncOp) Apply(c *Ctx) error                        { return o.ApplyFn(c) }
func (o *FuncOp) Restore(c *Ctx, raw json.RawMessage) error { return o.RestoreFn(c, raw) }
func (o *FuncOp) Current(c *Ctx) string {
	if o.CurFn == nil {
		return ""
	}
	return o.CurFn(c)
}
