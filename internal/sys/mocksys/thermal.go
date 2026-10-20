package mocksys

import "axion/internal/sys"

// ProcessorPower implements sys.Native.
func (n *Native) ProcessorPower() ([]sys.ProcPower, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ProcPowersErr != nil {
		return nil, n.ProcPowersErr
	}
	if n.ProcPowers != nil {
		return append([]sys.ProcPower(nil), n.ProcPowers...), nil
	}
	out := make([]sys.ProcPower, 16)
	for i := range out {
		out[i] = sys.ProcPower{MaxMhz: 4200, CurrentMhz: 4200, MhzLimit: 4200}
	}
	return out, nil
}
