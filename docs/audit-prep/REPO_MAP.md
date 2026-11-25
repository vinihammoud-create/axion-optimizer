# Mapa do repositório para a auditoria externa (base: v0.9.1, commit b48f356)

Leia este arquivo antes de abrir qualquer código. Ele diz o que o projeto é, onde cada coisa fica, o que já foi conferido e o que as ferramentas automáticas já encontraram. O objetivo é você não gastar leitura com o que já está resolvido.

## O que é

Axion Optimizer: otimizador para Windows 10/11 focado em jogos. Analisa o PC, mostra um diagnóstico com evidências, aplica ajustes com desfazer exato e mede o resultado. Roda elevado (`requireAdministrator` em `build/axion.manifest:8`).

- Backend em Go 1.24, sem cgo. Dependências copiadas em `third_party/` (não audite essa pasta).
- Interface em HTML/CSS/JS puro, embutida (`internal/web/static/`), servida por um servidor local em 127.0.0.1 e aberta numa janela WebView2.
- Código próprio: cerca de 58 mil linhas em `internal/` e `cmd/`, mais 20 mil de testes.

## Onde fica cada coisa

| Pasta | Conteúdo |
|---|---|
| `cmd/axion/` | Entrada do programa, janela, processo filho de coleta (`collect_windows.go`) |
| `internal/collect/`, `internal/facts/` | O que é lido do PC e como é guardado |
| `internal/diag/` | Regras do diagnóstico; a lista principal é `rules` em `diag.go:598` |
| `internal/tweaks/` | Motor de aplicar e desfazer: `engine.go`, `ops.go`, `journal.go` |
| `internal/catalog/` | Os ajustes; recusas em `excluded.go` |
| `internal/perf/`, `internal/app/perf.go` | Medição de FPS com o PresentMon e a estatística |
| `internal/sys/winsys/` | Acesso real ao Windows; `internal/sys/mocksys/` é o PC simulado dos testes |
| `internal/app/server.go` | Servidor local e rotas da API |
| `internal/programs/`, `internal/cleanup/` | Desinstalação, sobras, quarentena, limpeza |

## Regras do projeto (não são achados)

- Sem driver de kernel. Sem PowerShell nem arquivo de lote no produto.
- Nada é injetado em jogo.
- Todo ajuste tem fonte lida, risco declarado e desfazer exato.
- Limites escolhidos pelo projeto são chamados de "critério do Axion".

## Já conferido no código (não reporte de novo)

| Tema | Onde |
|---|---|
| Dados em `%ProgramData%\Axion`, com ACL só de administradores | `internal/sys/sys.go:1071`, `internal/sys/winsys/secure.go` |
| Pasta de dados do WebView2 fora de Program Files | `cmd/axion/main_windows.go:194` |
| Sem WebView2, a interface abre no navegador | `cmd/axion/main_windows.go:214` |
| PresentMon sob demanda, saída em arquivo (não em stdout), sem serviço | `internal/app/perf.go:405` |
| PresentMon e `WebView2Loader.dll` conferidos por SHA-256 | `internal/app/perf.go:87`, `cmd/axion/main_windows.go:276` |
| Snapshot de registro guarda se o valor e a chave existiam | `internal/tweaks/ops.go:79` |
| Histórico assinado com HMAC-SHA256 | `internal/tweaks/journal.go:105` |
| Servidor local: token por execução, comparação em tempo constante, conferência de Host e Origin | `internal/app/server.go:672-695` |
| Nenhuma chamada a PowerShell no produto | busca em `internal/` e `cmd/` |

## O que as ferramentas já encontraram (não repita; use como pista)

Execução de 25/11/2026 no GitHub Actions (`windows-latest`), fluxo `analise.yml`.

- **gitleaks:** nenhum segredo no histórico.
- **go test -race:** nenhuma corrida de dados nos 28 pacotes. Um teste falha só com o detector ligado: `TestCaptureRecordsConditionsAndSensors` (`internal/app/perfsensors_test.go:145`, sensores da GPU vazios). Não se sabe ainda se é só tempo do teste ou uma janela real em que a amostra de sensores se perde.
- **govulncheck:** 34 vulnerabilidades da biblioteca padrão alcançáveis, todas do Go 1.24.13 (sem suporte). A correção é subir a versão do Go e já está planejada.
- **golangci-lint:** 539 apontamentos. `staticcheck` não achou bug (só estilo). `errcheck` (171) é quase todo fechamento de handle. `gosec` (323) é quase todo `unsafe` e conversão de inteiro em chamadas Win32. Os de maior sinal, ainda não avaliados:

| Arquivo:linha | Regra | Apontamento |
|---|---|---|
| `internal/sys/winsys/secure.go:335` | G122 | caminho sujeito a troca por link dentro de `filepath.Walk` |
| `internal/app/server.go:641` | G705 | possível XSS por dado externo |
| `internal/app/server.go:706` | G706 | possível injeção em log |
| `internal/sys/winsys/native.go:708`, `715`, `722` | G204 | processo iniciado com argumento variável |
| `internal/sys/winsys/runner.go:166` | G204 | idem |
| `cmd/axion/main_windows.go:414` | G204 | `explorer.exe` com URL |
| `internal/sys/etw/modules.go:43-53` | G602 | índice de fatia possivelmente fora do limite |
| `internal/app/cs2preset.go:211`, `internal/diag/diag.go:350` | G602 | idem |
| `internal/sys/winsys/native.go:1085` | G404 | gerador aleatório fraco |
| `internal/sys/winsys/registry.go:110` | ineffassign | atribuição sem efeito a `typ` |

## Como rodar os testes (opcional)

```sh
export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off CGO_ENABLED=0
go test ./internal/...
```

Precisa do Go 1.24.7 ou mais novo. Os testes usam o PC simulado e não tocam na máquina. Código com `//go:build windows` não compila fora do Windows; para ele, a auditoria é por leitura.

## Limite desta auditoria

Você roda em Linux. Registro, UAC, WebView2 e PresentMon reais não existem aí. Quando um achado depender de comportamento real do Windows, marque-o como "validar no Windows" em vez de afirmar.
