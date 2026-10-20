//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"axion/internal/sys"
)

var (
	modWintrust                      = windows.NewLazySystemDLL("wintrust.dll")
	procWTHelperProvDataFromState    = modWintrust.NewProc("WTHelperProvDataFromStateData")
	procWTHelperGetProvSignerFromCha = modWintrust.NewProc("WTHelperGetProvSignerFromChain")
	procWTHelperGetProvCertFromChain = modWintrust.NewProc("WTHelperGetProvCertFromChain")
)

// VerifySignature implements sys.Native: WinVerifyTrust with the generic
// Authenticode policy (the same check Windows does before running a
// downloaded file), revocation checked for the whole chain except the root,
// then the leaf certificate's display name.
func (n *Native) VerifySignature(path string) (sys.Signature, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return sys.Signature{}, err
	}
	file := &windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: p}
	data := &windows.WinTrustData{
		Size:                            uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_WHOLECHAIN,
		UnionChoice:                     windows.WTD_CHOICE_FILE,
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		ProvFlags:                       windows.WTD_REVOCATION_CHECK_CHAIN_EXCLUDE_ROOT,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(file),
	}
	verr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	sig := sys.Signature{Trusted: verr == nil}
	if verr != nil {
		sig.Error = trustError(verr)
		var en windows.Errno
		if errors.As(verr, &en) && (uint32(en) == 0x80092013 || uint32(en) == 0x80092012) {
			sig.RevocationOffline = true // CRYPT_E_REVOCATION_OFFLINE / CRYPT_E_NO_REVOCATION_CHECK
		}
	}
	if data.StateData != 0 {
		sig.Signer = signerName(data.StateData)
	}
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	return sig, nil
}

// cryptProviderCert is the head of CRYPT_PROVIDER_CERT (wintrust.h).
type cryptProviderCert struct {
	cbStruct uint32
	_        uint32
	pCert    *windows.CertContext
}

// signerName reads the leaf certificate of the first signer from the
// WinVerifyTrust state.
func signerName(state windows.Handle) string {
	prov, _, _ := procWTHelperProvDataFromState.Call(uintptr(state))
	if prov == 0 {
		return ""
	}
	sgnr, _, _ := procWTHelperGetProvSignerFromCha.Call(prov, 0, 0, 0)
	if sgnr == 0 {
		return ""
	}
	pc, _, _ := procWTHelperGetProvCertFromChain.Call(sgnr, 0)
	if pc == 0 {
		return ""
	}
	// pc points into memory owned by wintrust.dll (not the Go heap).
	cert := (*cryptProviderCert)(*(*unsafe.Pointer)(unsafe.Pointer(&pc))).pCert
	if cert == nil {
		return ""
	}
	buf := make([]uint16, 256)
	nch := windows.CertGetNameString(cert, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], uint32(len(buf)))
	if nch <= 1 {
		return ""
	}
	return windows.UTF16ToString(buf[:nch])
}

func trustError(err error) string {
	var en windows.Errno
	if errors.As(err, &en) {
		switch uint32(en) {
		case uint32(windows.TRUST_E_NOSIGNATURE):
			return "arquivo sem assinatura digital"
		case uint32(windows.TRUST_E_BAD_DIGEST):
			return "o arquivo foi alterado depois de assinado"
		case uint32(windows.CERT_E_REVOKED):
			return "o certificado do assinante foi revogado"
		case uint32(windows.CERT_E_EXPIRED):
			return "certificado expirado sem carimbo de tempo válido"
		case uint32(windows.CERT_E_UNTRUSTEDROOT), uint32(windows.CERT_E_CHAINING):
			return "a cadeia do certificado não leva a uma autoridade confiável"
		case 0x80092013, 0x80092012: // CRYPT_E_REVOCATION_OFFLINE, CRYPT_E_NO_REVOCATION_CHECK
			return "não foi possível checar a revogação do certificado (servidor de certificados inacessível)"
		}
		return fmt.Sprintf("assinatura inválida (0x%08X)", uint32(en))
	}
	return err.Error()
}

// StartProgram implements sys.Native (ShellExecute "open", no wait).
func (n *Native) StartProgram(path, args string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("caminho de programa não aceito: %q", path)
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return fmt.Errorf("programa não encontrado: %s", path)
	}
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(path)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(path))
	var a *uint16
	if args != "" {
		a, _ = windows.UTF16PtrFromString(args)
	}
	return windows.ShellExecute(0, verb, file, a, dir, windows.SW_SHOWNORMAL)
}
