# Bloco B: o desfazer é exato e o processo elevado é seguro?

Cole este texto inteiro numa conversa nova do OpenHands, com o repositório `vinihammoud-create/axion-optimizer` no branch `audit/v0.9.1-baseline`.

---

Você é um auditor externo de código. Seu trabalho é **somente leitura**: não edite arquivos, não faça commit, push nem pull request. A única saída é um relatório em Markdown na sua resposta final.

Comece lendo `docs/audit-prep/REPO_MAP.md`. Ele lista o que já foi conferido e o que as ferramentas automáticas já acharam; não repita esses itens, mas use a tabela de apontamentos do `gosec` como pista.

## As duas perguntas

1. **Desfazer exato.** O Axion promete que cada ajuste volta ao estado idêntico ao anterior, inclusive quando o valor não existia. Em que situação essa promessa quebra?
2. **Processo elevado.** O Axion roda como administrador e lê arquivos e chaves que um programa sem administrador pode escrever. Onde um programa comum consegue fazer o Axion agir por ele?

## O que ler

- `internal/tweaks/` inteiro: `engine.go`, `ops.go`, `journal.go`, `compare.go`, `model.go`
- `internal/sys/winsys/registry.go`, `runner.go`, `secure.go` e `native.go` nas linhas 700 a 725
- `internal/app/server.go`
- `cmd/axion/main_windows.go` e `cmd/axion/collect_windows.go`
- `internal/programs/quarantine.go`, `leftovers.go`, `programs.go`
- `internal/cleanup/cleanup.go` e os arquivos `link_*.go`, `remove_*.go`
- `internal/games/cs2write.go` e `vdf.go`
- `internal/app/perf.go`, linhas 69 a 100 (cópia do PresentMon)

Não leia `third_party/`, `internal/diag/` nem `internal/perf/` (são do Bloco A).

## Hipóteses a verificar

Cada uma é uma suspeita, não um fato. Confirme ou descarte com `arquivo:linha`.

### Desfazer

- **B1. Queda no meio.** Se o processo morre entre gravar no Windows e gravar no histórico (ou o contrário), o que sobra? Existe um estado em que o ajuste está aplicado e o histórico não sabe, ou em que o histórico manda desfazer algo que não foi feito? Confira a ordem das gravações e o que `wal_test.go` cobre.
- **B2. Lote parcial.** Num lote de vários ajustes em que o terceiro falha, os dois primeiros ficam registrados e desfazíveis?
- **B3. Fidelidade do registro.** O snapshot preserva o tipo exato (`REG_DWORD`, `REG_QWORD`, `REG_SZ`, `REG_EXPAND_SZ`, `REG_MULTI_SZ`, `REG_BINARY`)? Trata diferente "valor não existia", "chave não existia" e "chave ancestral criada pelo Axion" (`ops.go:79`, `ops.go:181`)? Um `REG_EXPAND_SZ` volta como `REG_EXPAND_SZ`?
- **B4. Alguém mexeu no meio.** Se outro programa alterou o valor depois do Axion, desfazer sobrescreve a mudança desse programa sem avisar?
- **B5. Tipos de operação sem prova.** Para cada tipo (registro, serviço, plano de energia, tarefa agendada, BCD, perfil NVIDIA, `cs2_video.txt`, DNS, inicialização): existe desfazer exato e um teste que o prove? Liste os tipos sem teste.
- **B6. Histórico não verificável.** Quando a assinatura HMAC não confere (`journal.go:178`, `engine.go:825`), o que ainda pode ser desfeito e com que dado? Onde fica a chave e quem pode lê-la?
- **B7. Conta diferente.** Aberto com outra conta de administrador, os ajustes por usuário vão para o perfil de quem está usando o PC. O desfazer volta para o mesmo perfil?

### Processo elevado

- **B8. Servidor local.** `server.go:672-695` confere Host, Origin e token. Existe rota que escreve e escapa dessa conferência? A troca do código de uso único pelo token (`server.go:114-129`) pode ser disparada por uma página aberta no navegador do usuário? Avalie os apontamentos em `server.go:641` (G705) e `server.go:706` (G706).
- **B9. Arquivos que um programa comum controla.** O Axion lê: a pasta do WebView2 em `%LOCALAPPDATA%`, os `.vdf` da Steam, o `cs2_video.txt`, os logs do OBS, comandos de desinstalação gravados em `HKCU`. Algum desses dados chega a um caminho de escrita, de execução ou de exclusão sem validação?
- **B10. Processos iniciados.** Para cada ponto em que um processo é iniciado (`native.go:708`, `715`, `722`; `runner.go:166`; `main_windows.go:414`; `collect_windows.go:89`): o executável vem de caminho absoluto dentro das pastas do Windows? Algum argumento vem de dado que um programa comum controla?
- **B11. Troca por link.** A quarentena e a limpeza recusam junção e link simbólico. Existe intervalo entre conferir e agir em que o caminho pode ser trocado? Avalie o apontamento em `secure.go:335` (G122).
- **B12. Pasta de dados criada antes.** Se um programa comum cria `%ProgramData%\Axion` antes da primeira execução, o Axion detecta e renomeia. Há intervalo em que ele já confia na pasta?
- **B13. Carga de DLL.** O `WebView2Loader.dll` é conferido por hash antes de carregar. Entre conferir e carregar, o arquivo pode ser trocado? Ele é carregado por caminho absoluto?

## Formato do relatório

Máximo de 300 linhas. Não cole funções inteiras; cite `arquivo:linha` e, se preciso, até 10 linhas de código.

Comece com um parágrafo de veredito geral para cada uma das duas perguntas. Depois uma tabela com uma linha por achado:

| ID | Severidade | Arquivo:linha | O que acontece | Confiança | Critério de aceitação |
|---|---|---|---|---|---|

- **ID:** B-01, B-02...
- **Severidade:** crítica, alta, média ou baixa. Para os de processo elevado, diga o que o atacante precisa já ter (por exemplo, "rodar código como o usuário, sem administrador").
- **Confiança:** alta (li o caminho inteiro), média (li parte) ou baixa (inferência).
- **Critério de aceitação:** como um teste automático provaria que foi corrigido.

Depois da tabela, uma seção curta "Descartado" com as hipóteses que você verificou e não se confirmaram, com `arquivo:linha`. Feche com "Validar no Windows": o que não dá para afirmar só lendo.
