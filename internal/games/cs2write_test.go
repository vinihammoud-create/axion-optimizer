package games

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// realVideo is cs2_video.txt as CS2 writes it (CRLF, one TAB before the key
// and two between key and value), in the order of a real install.
var realVideo = func() string {
	kv := [][2]string{
		{"Version", "16"}, {"VendorID", "4318"}, {"DeviceID", "9708"},
		{"setting.cpu_level", "3"}, {"setting.gpu_mem_level", "3"}, {"setting.gpu_level", "3"}, {"setting.knowndevice", "0"},
		{"setting.defaultres", "1280"}, {"setting.defaultresheight", "960"},
		{"setting.refreshrate_numerator", "60"}, {"setting.refreshrate_denominator", "1"},
		{"setting.fullscreen", "1"}, {"setting.coop_fullscreen", "0"}, {"setting.nowindowborder", "0"},
		{"setting.mat_vsync", "0"}, {"setting.fullscreen_min_on_focus_loss", "1"}, {"setting.high_dpi", "0"},
		{"Autoconfig", "2"}, {"setting.shaderquality", "0"}, {"setting.r_texturefilteringquality", "0"},
		{"setting.msaa_samples", "2"}, {"setting.r_csgo_cmaa_enable", "0"},
		{"setting.videocfg_shadow_quality", "2"}, {"setting.videocfg_dynamic_shadows", "1"},
		{"setting.videocfg_texture_detail", "0"}, {"setting.videocfg_particle_detail", "0"},
		{"setting.videocfg_ao_detail", "0"}, {"setting.videocfg_hdr_detail", "-1"}, {"setting.videocfg_fsr_detail", "0"},
		{"setting.monitor_index", "0"}, {"setting.r_low_latency", "0"}, {"setting.aspectratiomode", "0"},
	}
	var sb strings.Builder
	sb.WriteString("\"video.cfg\"\r\n{\r\n")
	for _, p := range kv {
		sb.WriteString("\t\"" + p[0] + "\"\t\t\"" + p[1] + "\"\r\n")
	}
	sb.WriteString("}\r\n")
	return sb.String()
}()

func TestSetCS2VideoValueKeepsBytes(t *testing.T) {
	src := []byte(realVideo)
	out, err := SetCS2VideoValue(src, "setting.msaa_samples", "0")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(realVideo, "\"setting.msaa_samples\"\t\t\"2\"", "\"setting.msaa_samples\"\t\t\"0\"", 1)
	if string(out) != want {
		t.Fatalf("o arquivo mudou além do valor")
	}
	back, err := SetCS2VideoValue(out, "setting.msaa_samples", "2")
	if err != nil || !bytes.Equal(back, src) {
		t.Fatalf("voltar ao valor original não devolveu os bytes: %v", err)
	}
	if !bytes.Equal(src, []byte(realVideo)) {
		t.Fatal("a entrada foi modificada")
	}
}

func TestCS2VideoValueCaseInsensitive(t *testing.T) {
	v, ok, err := CS2VideoValue([]byte(realVideo), "SETTING.MSAA_SAMPLES")
	if err != nil || !ok || v != "2" {
		t.Fatalf("v=%q ok=%v err=%v", v, ok, err)
	}
	v, ok, _ = CS2VideoValue([]byte(realVideo), "setting.videocfg_hdr_detail")
	if !ok || v != "-1" {
		t.Errorf("valor negativo: %q %v", v, ok)
	}
}

func TestSetCS2VideoValueMissingKey(t *testing.T) {
	src := []byte(realVideo)
	out, err := SetCS2VideoValue(src, "setting.nao_existe", "1")
	if err == nil || out != nil {
		t.Fatalf("esperava erro, obtido %v", err)
	}
	if _, ok, err := CS2VideoValue(src, "setting.nao_existe"); ok || err != nil {
		t.Errorf("chave ausente: ok=%v err=%v", ok, err)
	}
}

func TestCS2VideoDuplicateKey(t *testing.T) {
	src := strings.Replace(realVideo, "}\r\n", "\t\"setting.msaa_samples\"\t\t\"4\"\r\n}\r\n", 1)
	_, _, err := CS2VideoValue([]byte(src), "setting.msaa_samples")
	if !errors.Is(err, ErrCS2VideoFormat) {
		t.Fatalf("chave repetida: %v", err)
	}
	if _, err := SetCS2VideoValue([]byte(src), "setting.msaa_samples", "0"); !errors.Is(err, ErrCS2VideoFormat) {
		t.Fatalf("gravar com chave repetida: %v", err)
	}
	// Another key of the same file is still fine.
	if _, ok, err := CS2VideoValue([]byte(src), "setting.mat_vsync"); err != nil || !ok {
		t.Errorf("outra chave: ok=%v err=%v", ok, err)
	}
}

func TestCS2VideoMalformed(t *testing.T) {
	cases := map[string]string{
		"valor sem aspas":    strings.Replace(realVideo, "\"setting.msaa_samples\"\t\t\"2\"", "\"setting.msaa_samples\"\t\t2", 1),
		"bloco aninhado":     strings.Replace(realVideo, "\t\"Autoconfig\"\t\t\"2\"\r\n", "\t\"sub\"\r\n\t{\r\n\t}\r\n", 1),
		"sem fechar":         strings.TrimSuffix(realVideo, "}\r\n"),
		"cabeçalho errado":   strings.Replace(realVideo, "video.cfg", "outro", 1),
		"texto depois do }":  realVideo + "\"x\" \"y\"\r\n",
		"sem chave de abrir": strings.Replace(realVideo, "{\r\n", "", 1),
	}
	for name, src := range cases {
		if _, _, err := CS2VideoValue([]byte(src), "setting.msaa_samples"); !errors.Is(err, ErrCS2VideoFormat) {
			t.Errorf("%s: esperava ErrCS2VideoFormat, obtido %v", name, err)
		}
		if _, err := SetCS2VideoValue([]byte(src), "setting.msaa_samples", "0"); err == nil {
			t.Errorf("%s: gravou mesmo assim", name)
		}
	}
}

func TestCS2VideoLFAndBOM(t *testing.T) {
	lf := strings.ReplaceAll(realVideo, "\r\n", "\n")
	bom := "\xEF\xBB\xBF" + realVideo
	for name, src := range map[string]string{"LF": lf, "BOM": bom, "BOM+LF": "\xEF\xBB\xBF" + lf} {
		out, err := SetCS2VideoValue([]byte(src), "setting.r_low_latency", "1")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := strings.Replace(src, "\"setting.r_low_latency\"\t\t\"0\"", "\"setting.r_low_latency\"\t\t\"1\"", 1)
		if string(out) != want {
			t.Errorf("%s: mudou além do valor", name)
		}
		back, _ := SetCS2VideoValue(out, "setting.r_low_latency", "0")
		if string(back) != src {
			t.Errorf("%s: não voltou ao original", name)
		}
	}
	// No final newline: the last line is the closing brace.
	if _, _, err := CS2VideoValue([]byte(strings.TrimRight(realVideo, "\r\n")), "setting.mat_vsync"); err != nil {
		t.Errorf("sem quebra de linha final: %v", err)
	}
}

func TestSetCS2VideoValueRejectsQuotes(t *testing.T) {
	for _, v := range []string{`1"`, "1\r", "1\n", `1\2`, strings.Repeat("9", 65)} {
		if _, err := SetCS2VideoValue([]byte(realVideo), "setting.msaa_samples", v); err == nil {
			t.Errorf("aceitou %q", v)
		}
	}
}

func TestParseCS2VideoStillReadsRealVideo(t *testing.T) {
	v, ok := ParseCS2Video(realVideo)
	if !ok || v.Width != 1280 || v.RefreshHz != 60 || v.Raw["setting.msaa_samples"] != "2" {
		t.Fatalf("%+v", v)
	}
}

func TestCS2VideoInCloud(t *testing.T) {
	const eight = `"730"
{
	"cs2_user_convars.vcfg" { "root" "0" "size" "10" }
	"cs2_user_keys.vcfg" { "root" "0" "size" "10" }
	"cfg/cs2_loadout_favorites.txt" { "root" "0" "size" "10" }
	"cfg/cs2_preferred_items.txt" { "root" "0" "size" "10" }
	"cfg/cs2_shuffle_slots.txt" { "root" "0" "size" "10" }
	"cfg/csgo_saved_item_shuffles.txt" { "root" "0" "size" "10" }
	"socache.dt" { "root" "0" "size" "10" }
	"voice_ban.dt" { "root" "0" "size" "10" }
}`
	root := `C:\Steam`
	rc := root + `\userdata\123\730\remotecache.vdf`
	for name, tc := range map[string]struct {
		content         *string
		inCloud, readOK bool
	}{
		"oito entradas": {content: ptr(eight), inCloud: false, readOK: true},
		"com o vídeo":   {content: ptr(strings.Replace(eight, "\"socache.dt\"", "\"cfg/cs2_video.txt\" { \"root\" \"0\" }\n\t\"socache.dt\"", 1)), inCloud: true, readOK: true},
		"sem o arquivo": {content: nil, inCloud: false, readOK: false},
		"sem o app 730": {content: ptr("\"440\" { }"), inCloud: false, readOK: false},
	} {
		m := newMemFS()
		if tc.content != nil {
			m.add(rc, *tc.content)
		}
		s := &Scanner{FS: m}
		in, ok := s.CS2VideoInCloud(root, "123")
		if in != tc.inCloud || ok != tc.readOK {
			t.Errorf("%s: (%v,%v), esperado (%v,%v)", name, in, ok, tc.inCloud, tc.readOK)
		}
	}
}

func ptr(s string) *string { return &s }
