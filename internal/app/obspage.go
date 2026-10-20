package app

import (
	"path/filepath"
	"time"

	"axion/internal/obs"
	"axion/internal/tweaks"
)

// ObsRow is a stop of the video read from a log, with what it says.
type ObsRow struct {
	obs.Session
	Analysis obs.Analysis `json:"analysis"`
}

// ObsView is the Live (OBS) page.
type ObsView struct {
	Found      bool            `json:"found"` // the OBS folder exists on this PC
	Dir        string          `json:"dir"`
	Rows       []ObsRow        `json:"rows"`       // newest first
	Latest     int             `json:"latest"`     // index in Rows of the session read (-1 none)
	UploadMbps float64         `json:"uploadMbps"` // from the newest network test (0 never measured)
	Honest     string          `json:"honest"`
	Criteria   string          `json:"criteria"`
	Privacy    string          `json:"privacy"`
	Sources    []tweaks.Source `json:"sources"`
}

// obsDir is where OBS Studio keeps its logs.
func (a *App) obsDir() string {
	return filepath.Join(a.P.Native.Paths().AppData, "obs-studio", "logs")
}

// uploadMbps is the upload speed of the newest network test that measured it.
func (a *App) uploadMbps() float64 {
	m := a.measured()
	if m.Net != nil && m.Net.UpMbps > 0 {
		return m.Net.UpMbps
	}
	if m.NetLatest != nil && m.NetLatest.UpMbps > 0 {
		return m.NetLatest.UpMbps
	}
	return 0
}

// OBS reads the logs of OBS Studio.
func (a *App) OBS() ObsView {
	v := ObsView{Dir: `%APPDATA%\obs-studio\logs`, Latest: -1, UploadMbps: a.uploadMbps(),
		Honest: "Quando uma transmissão ou gravação termina, o OBS escreve no log três números: os quadros que o codificador não conseguiu codificar a tempo (o processador, ou o codificador da placa de vídeo, estava sobrecarregado), " +
			"os que a placa de vídeo não conseguiu compor a tempo (ela estava no limite por causa do jogo) e, nas transmissões, os que a rede não conseguiu levar (a conexão não segurou a taxa de bits). O Axion lê esses números e diz qual dos três é o seu problema.",
		Criteria: "O Axion marca \"atenção\" a partir de 0,5% de quadros pulados pelo codificador, 1% de quadros atrasados pela placa de vídeo ou 0,5% de quadros perdidos na rede, e \"importante\" a partir de 2%, 5% e 2%. São critérios do Axion, não do OBS: " +
			"o próprio OBS avisa \"Encoding overloaded\" já a partir de 0,1%, o que numa transmissão longa costuma ser meia dúzia de quadros que ninguém vê. Com poucos quadros, uma falha pequena pesa muito na porcentagem.",
		Privacy: "O Axion lê só os arquivos de log da pasta acima, nunca as configurações do OBS (onde fica a chave da transmissão). Guarda apenas números e os nomes do codificador e da placa de vídeo, nenhuma linha do log. Nada sai do PC. O Streamlabs Desktop usa outra pasta e ainda não é lido.",
		Sources: []tweaks.Source{
			{Title: obs.SrcStreamKB.Title, URL: obs.SrcStreamKB.URL}, {Title: obs.SrcHAGS.Title, URL: obs.SrcHAGS.URL},
			{Title: obs.SrcLogLines.Title, URL: obs.SrcLogLines.URL}, {Title: obs.SrcLagForum.Title, URL: obs.SrcLagForum.URL}},
	}
	ss, found := obs.Read(a.obsDir(), time.Now())
	v.Found = found
	pick := obs.Pick(ss)
	for i := range ss {
		v.Rows = append(v.Rows, ObsRow{Session: ss[i], Analysis: obs.Analyze(&ss[i], v.UploadMbps)})
		if &ss[i] == pick {
			v.Latest = i
		}
	}
	return v
}
