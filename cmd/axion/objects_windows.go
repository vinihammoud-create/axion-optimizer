//go:build windows

package main

import (
	"log"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Axion's named objects (single-instance lock and finishing event in the
// Global namespace, reopen event in the session's) can be created by any
// program with any name. A program without administrator rights could create
// one of these names first and make Axion believe another copy is running,
// or signal it. So Axion creates them for Administrators and SYSTEM only,
// and trusts an existing one only when an administrator or the system owns it.

// adminOnlySA returns security attributes that make the Administrators group
// the owner and give Administrators and SYSTEM full access, nobody else any
// (nil if they cannot be built, which falls back to the token's defaults).
// The owner is explicit because the default owner is the user, not the
// group, with UAC off, on the built-in Administrator account or under the
// "object creator" owner policy.
func adminOnlySA(withOwner bool) *windows.SecurityAttributes {
	sddl := "D:P(A;;GA;;;BA)(A;;GA;;;SY)"
	if withOwner {
		sddl = "O:BA" + sddl
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		log.Printf("descritor de segurança dos objetos nomeados: %v", err)
		return nil
	}
	return &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
}

// createNamed creates (or opens) a named object with adminOnlySA. A token
// that cannot make the Administrators group the owner (not expected for an
// elevated Axion) gets the default owner instead of failing.
func createNamed(create func(sa *windows.SecurityAttributes) (windows.Handle, error)) (windows.Handle, error) {
	h, err := create(adminOnlySA(true))
	if h == 0 && err == windows.ERROR_INVALID_OWNER {
		h, err = create(adminOnlySA(false))
	}
	return h, err
}

// trustedObject reports whether the object behind h is owned by the
// Administrators group or by SYSTEM (the owner an elevated Axion gives the
// objects it creates). The handle needs READ_CONTROL access.
func trustedObject(h windows.Handle) bool {
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return false
	}
	return owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) || owner.IsWellKnown(windows.WinLocalSystemSid)
}
