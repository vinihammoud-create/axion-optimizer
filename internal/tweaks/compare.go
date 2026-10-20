package tweaks

import (
	"encoding/json"
	"reflect"
	"strings"

	"axion/internal/sys"
)

// snapComparer lets an op define which parts of a snapshot matter when
// deciding whether the current state still equals what Axion wrote.
type snapComparer interface {
	SnapEqual(a, b json.RawMessage) bool
}

// likeCapturer captures the same resource a snapshot refers to (e.g. the power
// scheme that was active at apply time, even if another scheme is active now).
type likeCapturer interface {
	CaptureLike(c *Ctx, like json.RawMessage) (json.RawMessage, error)
}

func canonicalEqual(a, b json.RawMessage) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return string(a) == string(b)
	}
	return reflect.DeepEqual(va, vb)
}

func snapEqual(op Op, a, b json.RawMessage) bool {
	if sc, ok := op.(snapComparer); ok {
		return sc.SnapEqual(a, b)
	}
	return canonicalEqual(a, b)
}

func captureLike(c *Ctx, op Op, like json.RawMessage) (json.RawMessage, error) {
	if lc, ok := op.(likeCapturer); ok {
		return lc.CaptureLike(c, like)
	}
	return op.Capture(c)
}

// SnapEqual compares only the value (key existence is incidental).
func (o *RegOp) SnapEqual(a, b json.RawMessage) bool {
	var sa, sb regSnap
	if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
		return false
	}
	return sa.Value.Equal(sb.Value)
}

// CaptureLike reads the values in the scheme recorded by the snapshot.
func (o *PowerOp) CaptureLike(c *Ctx, like json.RawMessage) (json.RawMessage, error) {
	var ref powerSnap
	if err := json.Unmarshal(like, &ref); err != nil || ref.Scheme == "" {
		return o.Capture(c)
	}
	if !schemeExists(c, ref.Scheme) {
		// The plan is gone: report the reference itself, so the entry is not
		// taken as changed by another program (Restore then has nothing to do).
		return like, nil
	}
	s := powerSnap{Scheme: ref.Scheme}
	if ref.AC != nil {
		v, err := c.P.Power.ReadValue(ref.Scheme, o.Sub, o.Setting, true)
		if err != nil {
			return nil, err
		}
		s.AC = &v
	}
	if ref.DC != nil {
		v, err := c.P.Power.ReadValue(ref.Scheme, o.Sub, o.Setting, false)
		if err != nil {
			return nil, err
		}
		s.DC = &v
	}
	return json.Marshal(s)
}

// SnapEqual compares AC/DC values.
func (o *PowerOp) SnapEqual(a, b json.RawMessage) bool {
	var sa, sb powerSnap
	if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
		return false
	}
	eq := func(x, y *uint32) bool {
		if x == nil || y == nil {
			return x == nil && y == nil
		}
		return *x == *y
	}
	return eq(sa.AC, sb.AC) && eq(sa.DC, sb.DC)
}

// SnapEqual compares only the previously active scheme.
func (o *SchemeOp) SnapEqual(a, b json.RawMessage) bool {
	var sa, sb schemeSnap
	if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
		return false
	}
	return equalFold(sa.Previous, sb.Previous)
}

// SnapEqual compares start type (running state changes on its own).
func (o *SvcOp) SnapEqual(a, b json.RawMessage) bool {
	var sa, sb svcSnap
	if json.Unmarshal(a, &sa) != nil || json.Unmarshal(b, &sb) != nil {
		return false
	}
	return sa.Exists == sb.Exists && sa.Start == sb.Start
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// CaptureLike reads the active plan. When the plan the snapshot recorded was
// deleted since, Balanced (what Restore activates in its place) counts as
// that plan, so the undo can be confirmed instead of staying pending forever.
func (o *SchemeOp) CaptureLike(c *Ctx, like json.RawMessage) (json.RawMessage, error) {
	var ref schemeSnap
	if err := json.Unmarshal(like, &ref); err == nil && ref.Previous != "" && !schemeExists(c, ref.Previous) {
		if act, err := c.P.Power.ActiveScheme(); err == nil && strings.EqualFold(act, sys.SchemeBalanced) {
			return like, nil
		}
	}
	return o.Capture(c)
}
