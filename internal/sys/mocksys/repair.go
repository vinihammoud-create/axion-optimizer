package mocksys

import (
	"strings"

	"axion/internal/sys"
)

// simulateRestoreHealth answers "DISM /Online /Cleanup-Image /RestoreHealth"
// with the exit code in Mock.RepairDISM and the message of a real DISM.
func (m *Mock) simulateRestoreHealth() sys.Result {
	m.mu.Lock()
	code, pt := m.RepairDISM, m.RepairPT
	m.mu.Unlock()
	bar := "[==========================100.0%==========================]"
	header := "Deployment Image Servicing and Management tool\r\nVersion: 10.0.26100.1\r\n\r\nImage Version: 10.0.26100.1\r\n\r\n"
	switch code {
	case 0, 3010:
		if pt {
			return sys.Result{ExitCode: code, Stdout: header + "[=====                      20.0%                           ]\r\n" + bar + "\r\nOperação de restauração concluída com êxito.\r\nA operação foi concluída com êxito.\r\n"}
		}
		return sys.Result{ExitCode: code, Stdout: header + "[=====                      20.0%                           ]\r\n" + bar + "\r\nThe restore operation completed successfully.\r\nThe operation completed successfully.\r\n"}
	case 740:
		return sys.Result{ExitCode: code, Stdout: "Error: 740\r\n\r\nElevated permissions are required to run DISM.\r\nUse an elevated command prompt to complete these tasks.\r\n"}
	}
	if pt {
		return sys.Result{ExitCode: code, Stdout: header + "Erro: 0x800f081f\r\n\r\nNão foi possível encontrar os arquivos de origem.\r\n"}
	}
	return sys.Result{ExitCode: code, Stdout: header + "Error: 0x800f081f\r\n\r\nThe source files could not be found.\r\n"}
}

// simulateSFC answers "sfc /scannow" with the message a real SFC prints for
// the situation in Mock.RepairSFC.
func (m *Mock) simulateSFC() sys.Result {
	m.mu.Lock()
	mode, pt := m.RepairSFC, m.RepairPT
	m.mu.Unlock()
	en := map[string]string{
		"":           "Windows Resource Protection did not find any integrity violations.",
		"repaired":   "Windows Resource Protection found corrupt files and successfully repaired them.\r\nFor online repairs, details are included in the CBS log file located at\r\nCBS.log.",
		"unrepaired": "Windows Resource Protection found corrupt files but was unable to fix some of them.\r\nFor online repairs, details are included in the CBS log file located at\r\nCBS.log.",
		"reboot":     "The maximum number of files have been queued for repair. A reboot is required to complete the repair of these files.",
		"pending":    "There is a system repair pending which requires reboot to complete.  Restart Windows and run sfc again.",
		"fail":       "Windows Resource Protection could not perform the requested operation.",
	}
	pb := map[string]string{
		"":           "A Proteção de Recursos do Windows não encontrou nenhuma violação de integridade.",
		"repaired":   "A Proteção de Recursos do Windows encontrou arquivos corrompidos e os reparou com êxito.\r\nPara reparos online, os detalhes são incluídos no arquivo de log CBS localizado em\r\nCBS.log.",
		"unrepaired": "A Proteção de Recursos do Windows encontrou arquivos corrompidos, mas não conseguiu corrigir alguns deles.\r\nPara reparos online, os detalhes são incluídos no arquivo de log CBS localizado em\r\nCBS.log.",
		"reboot":     "O número máximo de arquivos entraram na fila para reparo. É necessário reinicializar o computador para concluir o reparo destes arquivos.",
		"pending":    "Há um reparo de sistema pendente; é preciso reinicializar o computador para concluir a operação.  Reinicie o Windows e execute sfc novamente.",
		"fail":       "A Proteção de Recursos do Windows não pode executar a operação solicitada.",
	}
	msgs, start, done := en, "Beginning system scan.  This process will take some time.", "Verification 100% complete."
	if pt {
		msgs, start, done = pb, "Iniciando verificação de arquivos. O processo levará alguns minutos para ser concluído.", "Verificação 100% concluída."
	}
	msg, ok := msgs[mode]
	if !ok {
		msg = msgs[""]
	}
	code := 0
	if mode == "fail" || mode == "pending" {
		code = 1
	}
	return sys.Result{ExitCode: code, Stdout: strings.Join([]string{start, "", done, msg, ""}, "\r\n")}
}
