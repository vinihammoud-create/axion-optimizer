package drivers

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"axion/internal/sys"
)

// Verdicts for a Windows Update offer.
const (
	VerdictYes   = "recomendado"
	VerdictMaybe = "avaliar"
	VerdictNo    = "nao"
	// VerdictInstalled: the offered driver is the one in use (installed
	// after the search, by Axion or by Windows Update).
	VerdictInstalled = "instalado"
)

// Offer is a driver update offered by Windows Update, with Axion's reading
// of it for this PC.
type Offer struct {
	sys.DriverUpdate
	Component        string    `json:"component"`
	ComponentLabel   string    `json:"componentLabel"`
	ClassLabel       string    `json:"classLabel"`
	Version          string    `json:"version"` // from the title ("… - Net - 10.68.815.2023")
	Device           string    `json:"device"`
	InstalledVersion string    `json:"installedVersion"`
	InstalledDate    time.Time `json:"installedDate"`
	InstalledGeneric bool      `json:"installedGeneric"`
	Verdict          string    `json:"verdict"`
	Reasons          []string  `json:"reasons"`
	Select           bool      `json:"select"` // pre-selected on the page
	Status           string    `json:"status"` // "", "baixado", "instalado", "falhou"
	StatusDetail     string    `json:"statusDetail,omitempty"`
}

var classLabels = map[string]string{
	"display": "Vídeo", "net": "Rede", "media": "Áudio", "system": "Sistema/chipset", "bluetooth": "Bluetooth",
	"usb": "USB", "hidclass": "Dispositivo de entrada", "mouse": "Mouse", "keyboard": "Teclado", "firmware": "Firmware",
	"printer": "Impressora", "camera": "Câmera", "image": "Câmera/scanner", "softwarecomponent": "Componente de software",
	"extension": "Extensão de driver", "scsiadapter": "Controlador de armazenamento", "hdc": "Controlador de armazenamento",
	"monitor": "Monitor", "biometric": "Biometria", "sensor": "Sensor", "audioendpoint": "Áudio",
}

// ClassLabel translates a Windows driver class.
func ClassLabel(class string) string {
	if l, ok := classLabels[strings.ToLower(strings.TrimSpace(class))]; ok {
		return l
	}
	if class == "" {
		return "Outro"
	}
	return class
}

var classComponent = map[string]string{
	"display": "gpu", "net": "net", "media": "audio", "audioendpoint": "audio", "system": "chipset", "bluetooth": "bluetooth",
	"usb": "usb", "hidclass": "input", "mouse": "input", "keyboard": "input", "scsiadapter": "storage", "hdc": "storage",
}

var reTitleVersion = regexp.MustCompile(`(\d+(?:\.\d+){1,3})\s*$`)

// TitleVersion extracts the driver version from a Windows Update title.
func TitleVersion(title string) string {
	if m := reTitleVersion.FindStringSubmatch(strings.TrimSpace(title)); m != nil {
		return m[1]
	}
	return ""
}

// Evaluate builds the offers of the page from the Windows Update search and
// the installed drivers.
func Evaluate(updates []sys.DriverUpdate, installed []Installed) []Offer {
	out := make([]Offer, 0, len(updates))
	for _, u := range updates {
		o := Offer{DriverUpdate: u, ClassLabel: ClassLabel(u.Class), Version: TitleVersion(u.Title)}
		o.Component = classComponent[strings.ToLower(u.Class)]
		if d := Match(installed, u.HardwareID); d != nil {
			o.Device, o.InstalledVersion, o.InstalledDate, o.InstalledGeneric = d.Name, d.Version, d.Date, d.Generic
			if k := KindOf(*d); k != "" {
				o.Component = k
			}
		}
		if o.Device == "" {
			o.Device = u.Model
		}
		o.ComponentLabel = ComponentLabel(o.Component)
		judge(&o)
		o.Select = o.Verdict == VerdictYes && !o.Hidden
		out = append(out, o)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Hidden != out[j].Hidden {
			return !out[i].Hidden
		}
		return rank(out[i].Verdict) < rank(out[j].Verdict)
	})
	return out
}

func rank(v string) int {
	switch v {
	case VerdictYes:
		return 0
	case VerdictMaybe:
		return 1
	case VerdictNo:
		return 2
	}
	return 3
}

func judge(o *Offer) {
	class := strings.ToLower(o.Class)
	if o.InstalledVersion != "" && o.Version != "" && o.InstalledVersion == o.Version && !o.InstalledGeneric {
		o.Verdict = VerdictInstalled
		o.Reasons = []string{"Já é o driver em uso neste dispositivo."}
		return
	}
	older := !o.InstalledDate.IsZero() && !o.VerDate.IsZero() && o.VerDate.Before(o.InstalledDate)
	switch {
	case older:
		o.Verdict = VerdictNo
		o.Reasons = append(o.Reasons, "É mais antigo que o driver instalado ("+o.InstalledDate.Format("02/01/2006")+"). Instalar seria um retrocesso.")
	case class == "firmware":
		o.Verdict = VerdictMaybe
		o.Reasons = append(o.Reasons, "Atualiza o firmware (BIOS/UEFI ou de um componente). Pode corrigir falhas, mas não desligue o PC durante a instalação e, em notebook, deixe na tomada.")
	case class == "display":
		o.Verdict = VerdictMaybe
		o.Reasons = append(o.Reasons, "Para a placa de vídeo prefira o driver do fabricante (NVIDIA, AMD ou Intel): o do Windows Update costuma ser uma versão mais antiga, preparada pelo fabricante do PC.")
	case class == "softwarecomponent" || class == "extension":
		o.Verdict = VerdictMaybe
		o.Reasons = append(o.Reasons, "Complemento de outro driver (painel, serviço ou ajuste do fabricante). Instale se usa o recurso.")
	default:
		o.Verdict = VerdictYes
		switch {
		case o.InstalledGeneric:
			o.Reasons = append(o.Reasons, "Hoje o dispositivo usa o driver genérico do Windows; este é o driver do fabricante, certificado pela Microsoft para o seu hardware.")
		case !o.InstalledDate.IsZero():
			o.Reasons = append(o.Reasons, "Mais novo que o instalado ("+o.InstalledDate.Format("02/01/2006")+"), certificado pela Microsoft para o seu hardware.")
		default:
			o.Reasons = append(o.Reasons, "Driver certificado pela Microsoft para um dispositivo deste PC.")
		}
	}
	if o.BrowseOnly {
		o.Reasons = append(o.Reasons, "Atualização opcional: o Windows não instala sozinho.")
	} else {
		o.Reasons = append(o.Reasons, "O Windows Update instalaria esta sozinho em algum momento.")
	}
	if o.RebootBehavior == 1 {
		o.Reasons = append(o.Reasons, "Exige reiniciar.")
	}
}
