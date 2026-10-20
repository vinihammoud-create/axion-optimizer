package catalog_test

import (
	"context"
	"fmt"
	"testing"

	"axion/internal/collect"
	"axion/internal/facts"
	"axion/internal/sys"
	"axion/internal/tweaks"
)

// Win32PrioritySeparation (bits AABBCC): a fixed interval (BB = 10) or CC = 00
// takes the foreground boost away (Microsoft cc976120); fix.priosep must see
// all of them, including the 0x1A some optimizers write, and leave the others alone.
func TestPrioSepFixedIntervals(t *testing.T) {
	for _, v := range []int{0x1A, 0x29, 0x2A, 0x18, 0x14} {
		v := v
		t.Run(fmt.Sprintf("sem-vantagem-0x%X", v), func(t *testing.T) {
			m, _, e := setup(t)
			_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, "Win32PrioritySeparation", mocksysDW(uint32(v)))
			f := collect.Collect(context.Background(), m.Platform, nil)
			tw := e.Catalog.Get("fix.priosep")
			if ok, why := tw.IsApplicable(f); !ok {
				t.Fatalf("0x%X tira a vantagem do primeiro plano e deveria se aplicar: %s", v, why)
			}
			if tw.Recommendation(f).Level != tweaks.RecYes {
				t.Errorf("deveria ser recomendada: %+v", tw.Recommendation(f))
			}
			roundTrip(t, m, f, e, tw)
		})
	}
	for _, v := range []int{0x02, 0x26, 0x16, 0x0E} {
		v := v
		t.Run(fmt.Sprintf("com-vantagem-0x%X", v), func(t *testing.T) {
			m, _, e := setup(t)
			_ = m.R.SetValue(`HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, "Win32PrioritySeparation", mocksysDW(uint32(v)))
			f := collect.Collect(context.Background(), m.Platform, nil)
			if ok, _ := e.Catalog.Get("fix.priosep").IsApplicable(f); ok {
				t.Errorf("0x%X mantém o primeiro plano com mais tempo: não deveria se aplicar", v)
			}
		})
	}
}

// fix.net.throttling puts NetworkThrottlingIndex back to the Windows default
// and undoes to exactly what was there.
func TestNetThrottlingRoundTrip(t *testing.T) {
	m, _, e := setup(t)
	tw := e.Catalog.Get("fix.net.throttling")
	if tw == nil {
		t.Fatal("fix.net.throttling não existe")
	}
	f := collect.Collect(context.Background(), m.Platform, nil)
	if ok, _ := tw.IsApplicable(f); ok {
		t.Fatal("no padrão do Windows (10) não deveria se aplicar")
	}
	_ = m.R.SetValue(facts.KeySystemProfile, "NetworkThrottlingIndex", mocksysDW(0xFFFFFFFF))
	f = collect.Collect(context.Background(), m.Platform, nil)
	if ok, why := tw.IsApplicable(f); !ok {
		t.Fatalf("0xFFFFFFFF deveria se aplicar: %s", why)
	}
	if tw.Recommendation(f).Level != tweaks.RecYes {
		t.Errorf("deveria ser recomendada: %+v", tw.Recommendation(f))
	}
	res := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{})
	if len(res) != 1 || res[0].Outcome != tweaks.OutApplied {
		t.Fatalf("aplicar: %+v", res)
	}
	if v, _ := m.R.GetValue(facts.KeySystemProfile, "NetworkThrottlingIndex"); v.Int != 10 {
		t.Errorf("depois de aplicar o valor deveria ser 10, está %d", v.Int)
	}
	f = collect.Collect(context.Background(), m.Platform, nil)
	rev := e.RevertTweaks(context.Background(), f, []string{tw.ID}, tweaks.RevertOpts{})
	if len(rev) != 1 || rev[0].Outcome != tweaks.OutReverted {
		t.Fatalf("desfazer: %+v", rev)
	}
	if v, _ := m.R.GetValue(facts.KeySystemProfile, "NetworkThrottlingIndex"); v.Int != 0xFFFFFFFF {
		t.Errorf("desfazer deveria voltar ao valor anterior (0xFFFFFFFF), está %d", v.Int)
	}
	roundTripFresh(t, tw)
}

// roundTripFresh runs the generic apply/verify/revert proof on a new machine
// with the throttling off.
func roundTripFresh(t *testing.T, tw *tweaks.Tweak) {
	t.Helper()
	m, _, e := setup(t)
	_ = m.R.SetValue(facts.KeySystemProfile, "NetworkThrottlingIndex", mocksysDW(0xFFFFFFFF))
	f := collect.Collect(context.Background(), m.Platform, nil)
	roundTrip(t, m, f, e, e.Catalog.Get(tw.ID))
}

// XblGameSave and TabletInputService, which debloat packs turn off, are
// brought back to Manual by fix.services.apps, and undo disables them again.
func TestResidueServicesRoundTrip(t *testing.T) {
	prepare := func(t *testing.T) (*tweaks.Engine, *facts.Facts, func(string) sys.ServiceStart) {
		m, _, e := setup(t)
		m.S.Add("XblGameSave", "Xbox Live Game Save", sys.StartDisabled, false)
		m.S.Add("TabletInputService", "Touch Keyboard and Handwriting Panel Service", sys.StartDisabled, false)
		f := collect.Collect(context.Background(), m.Platform, nil)
		start := func(name string) sys.ServiceStart {
			info, _ := m.S.Query(name)
			return info.Start
		}
		return e, f, start
	}
	t.Run("aplica", func(t *testing.T) {
		e, f, start := prepare(t)
		tw := e.Catalog.Get("fix.services.apps")
		if ok, why := tw.IsApplicable(f); !ok {
			t.Fatalf("deveria se aplicar: %s", why)
		}
		res := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{})
		if len(res) != 1 || res[0].Outcome != tweaks.OutApplied {
			t.Fatalf("aplicar: %+v", res)
		}
		for _, name := range []string{"XblGameSave", "TabletInputService"} {
			if got := start(name); got != sys.StartManual {
				t.Errorf("%s: início %v depois de aplicar, esperava Manual", name, got)
			}
		}
	})
	t.Run("desfaz", func(t *testing.T) {
		e, f, start := prepare(t)
		tw := e.Catalog.Get("fix.services.apps")
		if res := e.Apply(context.Background(), f, []string{tw.ID}, tweaks.ApplyOpts{}); len(res) != 1 || res[0].Outcome != tweaks.OutApplied {
			t.Fatalf("aplicar: %+v", res)
		}
		rev := e.RevertTweaks(context.Background(), f, []string{tw.ID}, tweaks.RevertOpts{})
		if len(rev) != 1 || rev[0].Outcome != tweaks.OutReverted {
			t.Fatalf("desfazer: %+v", rev)
		}
		for _, name := range []string{"XblGameSave", "TabletInputService"} {
			if got := start(name); got != sys.StartDisabled {
				t.Errorf("%s: início %v depois de desfazer, esperava Desativado", name, got)
			}
		}
	})
}
