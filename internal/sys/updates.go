package sys

import (
	"context"
	"time"
)

// DriverUpdate is one driver that Windows Update offers for this PC
// (IWindowsDriverUpdate, from a search with Type='Driver').
type DriverUpdate struct {
	UpdateID       string    `json:"updateId"` // Identity.UpdateID (GUID)
	Revision       int       `json:"revision"`
	Title          string    `json:"title"`
	Description    string    `json:"description,omitempty"`
	Class          string    `json:"class"`      // DriverClass (Display, Net, System, MEDIA, Firmware…)
	HardwareID     string    `json:"hardwareId"` // DriverHardwareID matched on this PC
	Manufacturer   string    `json:"manufacturer"`
	Model          string    `json:"model"`
	Provider       string    `json:"provider"`
	VerDate        time.Time `json:"verDate"` // DriverVerDate
	SizeBytes      int64     `json:"sizeBytes"`
	Downloaded     bool      `json:"downloaded"`
	Hidden         bool      `json:"hidden"`
	BrowseOnly     bool      `json:"browseOnly"`     // "optional update": Windows does not install it by itself
	RebootBehavior int       `json:"rebootBehavior"` // 0 never, 1 always requires, 2 can request
	SupportURL     string    `json:"supportUrl,omitempty"`
}

// Operation result codes of the Windows Update Agent (OperationResultCode).
const (
	WUNotStarted          = 0
	WUInProgress          = 1
	WUSucceeded           = 2
	WUSucceededWithErrors = 3
	WUFailed              = 4
	WUAborted             = 5
)

// UpdateResult is the per-update outcome of a download or installation.
type UpdateResult struct {
	UpdateID       string `json:"updateId"`
	Title          string `json:"title"`
	Code           int    `json:"code"`    // OperationResultCode
	HResult        int32  `json:"hresult"` // 0 on success
	RebootRequired bool   `json:"rebootRequired"`
}

// OK reports a successful operation.
func (r UpdateResult) OK() bool { return r.Code == WUSucceeded || r.Code == WUSucceededWithErrors }

// Updates is the Windows Update Agent, limited to drivers. Every change
// (download, install, hide) goes through Windows Update itself, so the
// packages are the ones Microsoft signed and distributes for this hardware.
type Updates interface {
	// SearchDrivers returns the driver updates not installed, hidden ones included.
	SearchDrivers(ctx context.Context) ([]DriverUpdate, error)
	// DownloadDrivers downloads the given updates (UpdateIDs).
	DownloadDrivers(ctx context.Context, ids []string) ([]UpdateResult, error)
	// InstallDrivers installs updates that were downloaded. reboot reports
	// whether Windows needs a restart to finish.
	InstallDrivers(ctx context.Context, ids []string) (results []UpdateResult, reboot bool, err error)
	// SetHidden hides (or shows again) an update in Windows Update, so that
	// neither Windows nor Axion offer it any more.
	SetHidden(ctx context.Context, id string, hidden bool) error
}

// Signature is the Authenticode verdict for a file (WinVerifyTrust).
type Signature struct {
	Trusted bool   `json:"trusted"`
	Signer  string `json:"signer"` // subject of the leaf certificate (e.g. "NVIDIA Corporation")
	Error   string `json:"error,omitempty"`
	// RevocationOffline: the signature could not be accepted only because
	// the certificate revocation servers were unreachable (try again later).
	RevocationOffline bool `json:"revocationOffline,omitempty"`
}

// Paths are well-known folders (empty when unknown).
type Paths struct {
	WindowsDir      string `json:"windowsDir"`
	WindowsTemp     string `json:"windowsTemp"`
	UserTemp        string `json:"userTemp"`
	ProgramData     string `json:"programData"`
	ProgramFiles    string `json:"programFiles"`
	ProgramFilesX86 string `json:"programFilesX86"`
	AppData         string `json:"appData"`      // roaming
	LocalAppData    string `json:"localAppData"` // local
	StartupUser     string `json:"startupUser"`
	StartupCommon   string `json:"startupCommon"`
	SystemDrive     string `json:"systemDrive"` // "C:"
	UserProfile     string `json:"userProfile"`
	// Personal are the user's own folders where Windows really keeps them
	// (Documents, Desktop... may be moved to another drive or to OneDrive).
	Personal []string `json:"personal,omitempty"`
	// User is the account (DOMAIN\user) whose profile and HKCU the per-user
	// paths and settings refer to: the person signed in at the console.
	User string `json:"user,omitempty"`
	// ElevatedAs is the account Axion runs as when it is not User (UAC was
	// answered with another administrator account); empty otherwise.
	ElevatedAs string `json:"elevatedAs,omitempty"`
}
