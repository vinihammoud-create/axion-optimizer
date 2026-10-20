package facts

import (
	"sort"
	"strings"
)

// EEEKeywords are the energy-efficient Ethernet properties: the standardized
// *EEE plus the vendor keywords Realtek/Intel drivers commonly expose for the
// same feature.
var EEEKeywords = []string{"*EEE", "EEE", "AdvancedEEE", "EnableGreenEthernet", "GigaLite", "PowerSavingMode", "EEELinkAdvertisement"}

// nicOwnTweak are the properties Axion handles in their own items (energy
// efficient Ethernet, interrupt moderation, speed and duplex).
func nicOwnTweak(kw string) bool {
	if strings.EqualFold(kw, "*InterruptModeration") || strings.EqualFold(kw, "*SpeedDuplex") {
		return true
	}
	for _, e := range EEEKeywords {
		if strings.EqualFold(kw, e) {
			return true
		}
	}
	return false
}

// NicRestorable reports whether "restore the driver defaults" covers a
// property. Left out: the network identity (MAC address, VLAN, jumbo frames
// set up for a local network), what only matters with the PC asleep
// (Wake-on-LAN and the standby offloads), USB power saving and the
// properties Axion handles in their own items.
func NicRestorable(kw string) bool {
	low := strings.ToLower(strings.TrimSpace(kw))
	switch {
	case low == "", nicOwnTweak(kw):
		return false
	case low == "networkaddress", low == "vlanid", low == "*vlanid", strings.Contains(low, "jumbo"):
		return false
	case strings.Contains(low, "wake"), strings.Contains(low, "wol"), strings.HasPrefix(low, "*pm"),
		strings.Contains(low, "pme"), strings.Contains(low, "powerdown"),
		strings.HasPrefix(low, "*devicesleep"), strings.HasPrefix(low, "*modernstandby"):
		return false
	case strings.HasPrefix(low, "*selectivesuspend"), strings.HasPrefix(low, "*ssidle"):
		return false
	}
	return true
}

// DefaultAccepted reports whether the driver lists its own default among
// the values it accepts (always true for properties without a list): a
// default outside the list is an INF mistake, and writing it is not safe.
func (k NicKeyword) DefaultAccepted() bool {
	if len(k.Values) == 0 {
		return true
	}
	for _, v := range k.Values {
		if v == k.Default {
			return true
		}
	}
	return false
}

// NicChange is an advanced property set to something other than the
// driver's default.
type NicChange struct {
	Keyword string `json:"keyword"`
	Name    string `json:"name"`
	// DriverName is the label the driver itself shows in Device Manager
	// (ParamDesc), so the user finds the option by the name on screen.
	DriverName string `json:"driverName,omitempty"`
	Current    string `json:"current"` // driver text of the value
	Default    string `json:"default"` // driver text of the default
	Note       string `json:"note,omitempty"`
}

// Label is the name to show: the driver's own (what Device Manager
// prints), then Axion's translation after a slash when they differ.
func (c NicChange) Label() string {
	d := strings.TrimSpace(c.DriverName)
	if d == "" || strings.EqualFold(d, c.Name) || strings.EqualFold(d, c.Keyword) {
		return c.Name
	}
	if strings.EqualFold(c.Name, c.Keyword) {
		return d
	}
	return d + " / " + c.Name
}

// NonDefaults lists the restorable properties of the adapter whose value
// differs from the driver's default, in a stable order.
func (a Adapter) NonDefaults() []NicChange {
	var out []NicChange
	for kw, k := range a.Keywords {
		if !k.NonDefault() || !NicRestorable(kw) || !k.DefaultAccepted() {
			continue
		}
		name, note := NicKeywordInfo(kw)
		if name == "" {
			name = k.Name
		}
		if name == "" {
			name = kw
		}
		out = append(out, NicChange{Keyword: kw, Name: name, DriverName: strings.TrimSpace(k.Name), Current: k.Label(k.Current), Default: k.Label(k.Default), Note: note})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Keyword) < strings.ToLower(out[j].Keyword) })
	return out
}

// nicInfo explains common advanced properties. The Wi-Fi ones follow Intel's
// "Advanced Wi-Fi Adapter Settings" article (000005585); the wired ones the
// standardized NDIS keywords documented by Microsoft.
var nicInfo = map[string][2]string{
	"roamaggressiveness":       {"Agressividade de roaming", "nível de sinal em que a placa começa a procurar outro ponto de acesso; padrão Intel: médio"},
	"roamingpreferredbandtype": {"Banda preferida", "preferência por 2,4 ou 5 GHz; padrão Intel: sem preferência"},
	"mimopowersavemode":        {"Economia de energia MIMO (SMPS)", "mantém antenas em repouso para economizar; padrão Intel: Auto SMPS"},
	"throughputboosterenabled": {"Throughput Booster", "rajadas de transmissão; a Intel avisa que pode reduzir a vazão dos outros aparelhos da rede; padrão: desligado"},
	"*packetcoalescing":        {"Agrupamento de pacotes", "reúne pacotes de broadcast/multicast para acordar menos a CPU (economia de energia); padrão: ligado"},
	"bgscanglobalblocking":     {"Bloqueio global de busca em segundo plano", "impede as buscas periódicas por outros pontos de acesso; padrão Intel: nunca"},
	"enablechload4apselection": {"Carga do canal na escolha do ponto de acesso", ""},
	"uapsdsupport":             {"Suporte a U-APSD", "economia de energia WMM; padrão Intel: desligado"},
	"fatchannelintolerant":     {"Intolerância a canal largo (40 MHz em 2,4 GHz)", ""},
	"ctstoitself":              {"Proteção de modo misto", ""},
	"*flowcontrol":             {"Controle de fluxo (802.3x)", ""},
	"*lsov2ipv4":               {"Descarga de segmentação TCP (LSO, IPv4)", ""},
	"*lsov2ipv6":               {"Descarga de segmentação TCP (LSO, IPv6)", ""},
	"*tcpchecksumoffloadipv4":  {"Descarga de checksum TCP (IPv4)", ""},
	"*tcpchecksumoffloadipv6":  {"Descarga de checksum TCP (IPv6)", ""},
	"*udpchecksumoffloadipv4":  {"Descarga de checksum UDP (IPv4)", ""},
	"*udpchecksumoffloadipv6":  {"Descarga de checksum UDP (IPv6)", ""},
	"*ipchecksumoffloadipv4":   {"Descarga de checksum IP (IPv4)", ""},
	"*rss":                     {"Receive Side Scaling (RSS)", "reparte a recepção entre núcleos da CPU"},
	"*numrssqueues":            {"Filas de RSS", ""},
	"*receivebuffers":          {"Buffers de recepção", ""},
	"*transmitbuffers":         {"Buffers de transmissão", ""},
	"*priorityvlantag":         {"Prioridade de pacote e VLAN", ""},
}

// NicKeywordInfo returns a Portuguese name and a short note for a known
// advanced property ("" when unknown).
func NicKeywordInfo(kw string) (name, note string) {
	v, ok := nicInfo[strings.ToLower(strings.TrimSpace(kw))]
	if !ok {
		return "", ""
	}
	return v[0], v[1]
}
