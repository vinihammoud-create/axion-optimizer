# Regras para agentes que executam tarefas neste repositório

Vale para qualquer agente de código (OpenHands ou outro). Quem escreve a tarefa e avalia o resultado é outro: você executa uma issue por vez e entrega um pull request.

## Antes de começar

1. Leia a issue inteira. Ela diz o que fazer, em quais arquivos e como provar.
2. Leia [`docs/COMO-MODIFICAR.md`](docs/COMO-MODIFICAR.md) (onde fica cada coisa) e, se a tarefa mexer num ajuste do catálogo, o [`REVISAO-DE-MUDANCAS.md`](REVISAO-DE-MUDANCAS.md).

## Regras do produto

- Sem driver de kernel. Sem PowerShell nem arquivo de lote no produto. Nada é injetado em jogo.
- Todo ajuste tem fonte aberta e lida, e desfazer que volta ao estado exato.
- Dado não lido não é "normal": leitura que falhou nunca vira estado verde nem alarme.
- Texto para o usuário em português do Brasil, com acentos, sem prometer o que não foi medido.
- Não altere `third_party/`, `.github/`, `.openhands/`, a versão (`cmd/axion/version.go`, `build/axion.manifest`), o `.syso` nem as imagens da marca.

## Como entregar

- **Prove antes de corrigir.** Escreva primeiro o teste do critério de aceitação e confirme que ele falha no `main`. Se não falhar, o problema descrito não existe: não mude nada, comente na issue o que você viu e pare.
- **Só o que a issue pede.** Não refatore, não renomeie e não "aproveite" para arrumar outra coisa. Achou outro problema? Descreva no pull request, sem corrigir.
- **Um branch e um pull request por issue,** contra `main`. Não mescle e não envie direto ao `main`.
- **Descrição do pull request:** o que mudou e por quê, a saída do teste antes (falhando) e depois (passando), e `Closes #N`.
- **Tarefa de auditoria** (a issue diz "somente leitura"): não mude código. O pull request traz só o relatório, no arquivo que a issue indicar.

## Verificação obrigatória

Go 1.27.4 ou mais novo (o `.openhands/setup.sh` instala). As dependências estão em `third_party/`; nada é baixado.

```sh
export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off CGO_ENABLED=0 GOTOOLCHAIN=local
gofmt -l internal cmd            # precisa sair vazio
go vet ./cmd/... ./internal/...
go test ./internal/<pacote>/...  # os pacotes que você tocou
```

Os testes rodam sobre o PC simulado (`internal/sys/mocksys`) e não tocam na máquina. Código com `//go:build windows` não compila fora do Windows: o CI (`windows-latest`) é quem confere esse código, então espere o resultado dele no pull request.
