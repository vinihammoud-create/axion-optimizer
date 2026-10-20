package repair

import "testing"

func TestParseSFC(t *testing.T) {
	cases := []struct {
		name string
		exit int
		out  string
		want Outcome
	}{
		{"en limpo", 0, "Beginning system scan.  This process will take some time.\r\nBeginning verification phase of system scan.\r\nVerification 100% complete.\r\nWindows Resource Protection did not find any integrity violations.", OK},
		{"pt limpo", 0, "Iniciando verificação de arquivos. O processo levará alguns minutos para ser concluído.\r\nVerificação 100% concluída.\r\nA Proteção de Recursos do Windows não encontrou nenhuma violação de integridade.", OK},
		{"en reparado", 0, "Windows Resource Protection found corrupt files and successfully repaired them.\r\nFor online repairs, details are included in the CBS log file located at\r\nCBS.log.", Repaired},
		{"pt reparado", 0, "A Proteção de Recursos do Windows encontrou arquivos corrompidos e os reparou com êxito.", Repaired},
		{"en sem conserto", 0, "Windows Resource Protection found corrupt files but was unable to fix some of them.", Unrepaired},
		{"pt sem conserto", 0, "A Proteção de Recursos do Windows encontrou arquivos corrompidos, mas não conseguiu corrigir alguns deles.", Unrepaired},
		{"en metadados", 0, "Windows Resource Protection did not find any integrity violations but cannot\r\nguarantee the integrity of the system because component metadata is corrupt.", Unrepaired},
		{"pt metadados", 0, "A Proteção de Recursos do Windows não encontrou violações de integridade, mas não pode\r\ngarantir a integridade do sistema porque os metadados do componente estão corrompidos.", Unrepaired},
		{"en pendente", 0, "There is a system repair pending which requires reboot to complete.  Restart Windows and run sfc again.", Reboot},
		{"pt pendente", 0, "Há um reparo de sistema pendente; é preciso reinicializar o computador para concluir a operação.  Reinicie o Windows e execute sfc novamente.", Reboot},
		{"en fila", 0, "The maximum number of files have been queued for repair. A reboot is required to complete the repair of these files.", Reboot},
		{"pt fila", 0, "O número máximo de arquivos entraram na fila para reparo. É necessário reinicializar o computador para concluir o reparo destes arquivos.", Reboot},
		{"en nao fez", 1, "Windows Resource Protection could not perform the requested operation.", Failed},
		{"pt nao fez", 1, "A Proteção de Recursos do Windows não pode executar a operação solicitada.", Failed},
		{"en servico", 1, "Windows Resource Protection could not start the repair service.", Failed},
		{"pt servico", 1, "A Proteção de Recursos do Windows não pôde iniciar o serviço de reparo.", Failed},
		{"en outra", 1, "Another servicing or repair operation is currently running.", Failed},
		{"pt outra", 1, "Há outro serviço ou operação de reparo em execução atualmente.", Failed},
		{"idioma desconhecido, saida 0", 0, "Windows-Ressourcenschutz hat keine Integritätsverletzungen gefunden.", Unknown},
		{"idioma desconhecido, saida 2", 2, "xyz", Failed},
	}
	for _, c := range cases {
		got, msg := ParseSFC(c.exit, c.out)
		if got != c.want {
			t.Errorf("%s: %s (%q), esperado %s", c.name, got, msg, c.want)
		}
		if msg == "" {
			t.Errorf("%s: sem texto", c.name)
		}
	}
}

func TestParseDISM(t *testing.T) {
	cases := []struct {
		name string
		exit int
		out  string
		want Outcome
	}{
		{"ok", 0, "[==========================100.0%==========================]\r\nThe restore operation completed successfully.\r\nThe operation completed successfully.", OK},
		{"ok pt", 0, "Operação de restauração concluída com êxito.\r\nA operação foi concluída com êxito.", OK},
		{"reinicio", ExitRebootRequired, "The operation completed successfully.", Reboot},
		{"origem en", 0x800f081f, "Error: 0x800f081f\r\nThe source files could not be found.", Failed},
		{"origem pt", 0x800f081f, "Erro: 0x800f081f\r\nNão foi possível encontrar os arquivos de origem.", Failed},
		{"download", 0x800f0915, "The source files could not be downloaded.", Failed},
		{"sem admin", 740, "Error: 740\r\nElevated permissions are required to run DISM.\r\nUse an elevated command prompt to complete these tasks.", Failed},
		{"outra manutencao", 0x800f0822, "The operation cannot be completed because Windows is currently performing another servicing operation.", Failed},
		{"codigo qualquer", 87, "Error: 87", Failed},
	}
	for _, c := range cases {
		got, msg := ParseDISM(c.exit, c.out)
		if got != c.want || msg == "" {
			t.Errorf("%s: %s (%q), esperado %s", c.name, got, msg, c.want)
		}
	}
	if _, msg := ParseDISM(0x800f081f, "xyz"); msg != "O DISM terminou com o código 0x800F081F." {
		t.Errorf("código em hexa: %q", msg)
	}
}

func TestPercent(t *testing.T) {
	for line, want := range map[string]float64{
		"[=====                      20.0%                           ]": 20,
		"Verification 45% complete.":                                    45,
		"Verificação 7% concluída.":                                     7,
		"[==========================100.0%==========================]":  100,
		"[==   3,8%   ]": 3.8,
	} {
		got, ok := Percent(line)
		if !ok || got != want {
			t.Errorf("%q: %v %v, esperado %v", line, got, ok, want)
		}
	}
	for _, line := range []string{"Beginning system scan.", "Version: 10.0.26100.1", "", "250% x"} {
		if _, ok := Percent(line); ok {
			t.Errorf("%q não tem progresso", line)
		}
	}
}
