# Axion Optimizer: instruções para o Claude Code

Otimizador para Windows 10/11 focado em jogos. Go no backend, interface em HTML/CSS/JS puro embutida no executável e aberta numa janela WebView2. O mapa de pastas e os comandos de build estão em [`docs/COMO-MODIFICAR.md`](docs/COMO-MODIFICAR.md); não repita aqui o que já está lá.

## Regras do produto

- **Sem driver de kernel.** Nada de WinRing0, PawnIO ou driver próprio. O que o Windows não informa sem driver, o Axion não lê.
- **Sem PowerShell nem arquivo de lote no produto.** Só registro, WMI, API do Windows e ferramentas do próprio Windows. O `build/build.ps1` é ferramenta de build, não produto.
- **Nada é injetado em jogo** nem desenhado sobre ele.
- **Só entra ajuste com fonte aberta e lida.** Siga o [`REVISAO-DE-MUDANCAS.md`](REVISAO-DE-MUDANCAS.md) e confira a lista de recusas em `internal/catalog/excluded.go`.
- **Desfazer exato.** Toda operação guarda o valor anterior (inclusive "não existia") e volta ao estado idêntico. O teste `TestApplyVerifyRevertEveryTweak` cobre todos os itens.
- **Dado não lido não é "normal".** Uma regra de diagnóstico só conclui algo quando a leitura respondeu: veja `Unread`, `TPMRead`, `ZonesRead` e `LimitRead` em `internal/facts/`. Leitura que falhou vira "não foi possível ler", nunca estado verde.
- **Texto em português do Brasil, com acentos,** sem prometer o que não foi medido. Limite escolhido pelo projeto se chama "critério do Axion".
- **Marca aprovada, não mexer:** `build/axion-mark.png`, `build/axion.ico`, `docs/img/axion-logo.jpg`, `docs/img/axion-mark.png`, `internal/web/static/logo-*.png`.
- **Não alterar sem pedido explícito:** `third_party/`, a versão (`cmd/axion/version.go`, `build/axion.manifest`) e o `.syso`.

## Papéis

- **O Claude Code planeja e avalia; não é o executor.** Escreve a tarefa como issue, com arquivos e critério de aceitação; depois lê o pull request pronto, confere contra o código e decide: mesclar, pedir ajuste ou recusar.
- **Quem executa é um agente externo** (OpenHands), acionado pela etiqueta `openhands` na issue. As regras dele estão em [`AGENTS.md`](AGENTS.md); o ambiente e a verificação automática, em `.openhands/`.
- **Quem controla é a nuvem:** o `main` só recebe mudança por pull request com o CI verde.
- **Na avaliação, desconfie do pull request:** o teste novo falha sem a correção? Algo fora do escopo mudou? As regras do produto continuam valendo?
- **Decide sozinho** o que é correção com teste que prova. **Decide com o dono** o que muda o que o usuário vê, o CI, a publicação de versão ou o gasto.
- **Economia:** neste notebook, nada de suíte inteira, `go vet ./...` nem agentes em paralelo sem pedido. Achado de segurança ainda sem correção não vai para issue pública.
- **Mudou o que o Windows faz?** Registre em `build/LEIA-ME.txt`, na seção "NOVO NA x.y".

## Verificação

```powershell
$env:GOFLAGS='-mod=mod'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:CGO_ENABLED='0'; $env:GOTOOLCHAIN='local'
gofmt -l internal cmd            # precisa sair vazio
go vet ./cmd/... ./internal/...
go test ./internal/<pacote>/...  # os pacotes que a mudança tocou
```

- Os testes rodam sobre o PC simulado (`internal/sys/mocksys`) e nunca escrevem na máquina real.
- A máquina de desenvolvimento é um notebook antigo: o `go vet` a frio leva cerca de 2 minutos e a suíte inteira, cerca de 3. Rode local só o que a mudança tocou e deixe a suíte inteira para o CI (`windows-latest`).
- Resultado do CI sem abrir o navegador: `gh pr checks <n>` e `gh run view <id> --log-failed`.
- Para ver a interface: `.\build\dev\preview.ps1` (abre sobre o PC simulado); capturas com `build\dev\shot.ps1`.

## O que não dá para provar neste ambiente

Jogo, placa de vídeo dedicada e anti-cheat reais não existem nem no notebook nem no runner da nuvem. O que depende deles (medição de FPS, perfil do driver NVIDIA, preset do CS2) fica "testado só no simulador", e o texto para o usuário precisa dizer isso.
