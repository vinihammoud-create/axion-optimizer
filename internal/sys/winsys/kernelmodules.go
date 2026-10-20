//go:build windows

package winsys

import "errors"

// KernelModules implements sys.Native: the file names of the loaded kernel
// modules. Read only. Without administrator rights the list comes back empty
// (the base addresses read as zero), which is reported as an error so nobody
// takes it for "no driver loaded".
func (n *Native) KernelModules() ([]string, error) {
	mods := kernelModules()
	if len(mods) == 0 {
		return nil, errors.New("lista de drivers do kernel indisponível (é preciso executar como administrador)")
	}
	out := make([]string, 0, len(mods))
	for _, m := range mods {
		out = append(out, m.Name)
	}
	return out, nil
}
