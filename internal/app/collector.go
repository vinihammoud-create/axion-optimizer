package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"

	"axion/internal/bcd"
	"axion/internal/collect"
	"axion/internal/facts"
)

// Collector gathers the machine profile. The Windows build sets one that runs
// the collection in a separate Axion process: a Windows component that fails
// inside a probe (a driver query, a service, a device) then ends that process,
// not the interface. nil collects in this process (tests, the mock).
type Collector func(ctx context.Context, skip []string, progress collect.Progress, mark func(part string, done bool)) (*facts.Facts, error)

// CollectCrash is a collection whose process ended without a result.
type CollectCrash struct {
	Part   string // step or part of a step that was running
	Detail string // exit code and the end of what the process wrote
}

func (e *CollectCrash) Error() string {
	if e.Part == "" {
		return "a coleta de informações foi encerrada antes de começar: " + e.Detail
	}
	return fmt.Sprintf("a coleta de informações foi encerrada durante \"%s\": %s", e.Part, e.Detail)
}

// maxCollectRetries bounds how many parts can be set aside in one analysis.
const maxCollectRetries = 4

// collectFacts gathers the profile. When the collection process is ended by a
// failing part, that part is remembered for this version of Axion and the
// collection runs again without it: the analysis goes on with one item
// missing (listed in the collection notices) instead of stopping.
func (a *App) collectFacts(ctx context.Context, c *JobCtl, progress collect.Progress) (*facts.Facts, error) {
	bcd.Invalidate(a.P) // this process reads the boot configuration too
	mark := func(part string, done bool) {
		if !done {
			log.Printf("análise:   %s", part)
		}
	}
	run := func() (*facts.Facts, error) {
		skip := a.skippedParts()
		if a.Collector == nil {
			set := map[string]bool{}
			for _, s := range skip {
				set[s] = true
			}
			return collect.CollectWith(ctx, a.P, progress, collect.Options{Skip: set, Mark: mark}), nil
		}
		return a.Collector(ctx, skip, progress, mark)
	}
	f, err := run()
	for try := 0; try < maxCollectRetries && err != nil && ctx.Err() == nil; try++ {
		var crash *CollectCrash
		if !errors.As(err, &crash) || crash.Part == "" {
			break
		}
		log.Printf("aviso: %v; repetindo sem essa parte", crash)
		if c != nil {
			c.Logf("aviso: \"%s\" encerrou a coleta neste PC e foi deixada de fora", crash.Part)
		}
		if !a.rememberSkip(crash.Part) {
			break // already skipped: something else is failing
		}
		f, err = run()
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// skippedParts lists the parts set aside for this version.
func (a *App) skippedParts() []string {
	s := a.Settings()
	if s.SkipVersion != a.Version {
		return nil
	}
	return s.SkipParts
}

// rememberSkip sets a part aside for this version. It reports false when the
// part was already set aside. A new version tries every part again.
func (a *App) rememberSkip(part string) bool {
	added := false
	err := a.UpdateSettings(func(s *Settings) {
		if s.SkipVersion != a.Version {
			s.SkipVersion, s.SkipParts = a.Version, nil
		}
		for _, p := range s.SkipParts {
			if p == part {
				return
			}
		}
		s.SkipParts = append(append([]string(nil), s.SkipParts...), part)
		sort.Strings(s.SkipParts)
		added = true
	})
	if err != nil {
		log.Printf("guardar a parte pulada: %v", err)
	}
	return added
}
