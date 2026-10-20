# Como contribuir

Obrigado pelo interesse. O Axion tem uma regra que vem antes de qualquer código: **só entra o que tem evidência**. A seção 1 explica o que isso significa na prática.

## 1. Antes de propor um ajuste novo

- **Leia o [`REVISAO-DE-MUDANCAS.md`](REVISAO-DE-MUDANCAS.md).** Todo ajuste precisa de uma fonte aberta e lida (documentação do fabricante ou medição pública) e de risco e evidência honestos. Também precisa de um desfazer que volte ao estado exato.
- **Confira a lista "O que não fazemos"** (`internal/catalog/excluded.go`). Se a ideia está lá, ela foi recusada de propósito, com o motivo.
- **Textos em português do Brasil**, sem prometer o que não foi medido. Os limites próprios do Axion são ditos como "critério do Axion".

## 2. Compilar e testar

Requisitos: Windows, Go 1.24 ou mais novo e Windows PowerShell. As dependências ficam em `third_party/`, então nada é baixado.

```powershell
$env:GOFLAGS='-mod=mod'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:CGO_ENABLED='0'
go vet ./cmd/... ./internal/...
go test ./internal/...
.\build\build.ps1            # gera dist\Axion.exe e o pacote .zip
```

Os testes usam um PC simulado (`internal/sys/mocksys`) e nunca escrevem no registro, nos serviços, nos drivers ou nos arquivos de jogos da máquina real.

Para ver a interface sem mexer no seu sistema, use `.\build\dev\preview.ps1`. Detalhes em [`docs/COMO-MODIFICAR.md`](docs/COMO-MODIFICAR.md).

## 3. Pull requests

- **Uma mudança por PR**, com o porquê na descrição.
- **A suíte inteira precisa passar** (`go test ./internal/...`) e o `gofmt -l internal cmd` precisa sair vazio.
- **Mudou o que o Windows faz?** Diga no `build/LEIA-ME.txt`, na seção "NOVO NA x.y".
