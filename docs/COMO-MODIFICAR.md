# Como modificar o Axion

O `Axion.exe` não se edita direto: a interface e as otimizações são compiladas dentro dele. Toda mudança é no código, e depois um comando gera o `.exe` novo (cerca de 1 minuto).

## Onde fica cada coisa

| O quê | Onde |
|---|---|
| Otimizações e correções (o catálogo) | `internal/catalog/*.go`, juntadas em `catalog.All()` (`common.go`) |
| O que o Axion recusa de propósito | `internal/catalog/excluded.go` |
| Regras do diagnóstico (os alertas) | `internal/diag/*.go`; a lista principal é `rules` em `diag.go` |
| Leitura do PC | `internal/collect/*.go` (o que é lido) e `internal/facts/*.go` (o que é guardado) |
| Acesso ao Windows | `internal/sys/winsys` (real) e `internal/sys/mocksys` (simulado, para testes) |
| Interface | `internal/web/static/app.js`, `app.css` e `index.html` |
| Servidor local e rotas da API | `internal/app/server.go` |
| Ícone, manifesto e versão do `.exe` | `build/axion-mark.png`, `build/axion.manifest`, gerados por `cmd/axion-rsrc` |
| Texto que vai no pacote | `build/LEIA-ME.txt` |

## Mudar a interface

1. Edite `internal/web/static/app.js` ou `app.css`.
2. Veja na hora, sem mexer no seu PC:
   ```powershell
   .\build\dev\preview.ps1
   ```
   O comando imprime um endereço `dev=...`. Abra-o para receber o link de uso único, que abre a interface sobre um PC simulado.
3. Para tirar fotos de várias páginas de uma vez, use o `build\dev\shot.ps1` (as instruções estão no topo do arquivo).

As cores e os tamanhos ficam nos tokens do começo do `app.css` (`:root`).

## Adicionar uma otimização

1. Ache o arquivo da categoria em `internal/catalog/` e copie um item parecido. Cada item é um `tweaks.Tweak` com:
   - `ID`, `Category`, `Title`, `Summary`, `Detail`: o que aparece na tela;
   - `Evidence` e `Sources`: o nível de evidência e as páginas abertas e lidas;
   - `Risk`, `Reboot` e `OneClick`;
   - `Applicable` e `Recommend`: quando o item vale e se é recomendado para o PC analisado;
   - `Ops`: o que é gravado. Use `tweaks.Reg(...)` (registro), `tweaks.Svc(...)` (serviço), `tweaks.Scheme(...)` (plano de energia) ou um `FuncOp` próprio.
2. Siga o `REVISAO-DE-MUDANCAS.md`: fonte lida, risco honesto, desfazer exato.
3. Rode os testes. O `TestApplyVerifyRevertEveryTweak` aplica e desfaz todos os itens no simulador; o `guard_test.go` barra o que contradiz "O que não fazemos" e os processos do sistema.

## Gerar o .exe

```powershell
.\build\build.ps1 -Version 0.9.1
```

O script roda `go vet` e os testes, gera os recursos (ícone e versão) e compila. O resultado fica em:
- `dist\Axion.exe`
- `dist\Axion-0.9.1-win64.zip`, com o `LEIA-ME.txt`, a matriz de evidências e o `SHA256SUMS.txt`.

Com `-SkipTests` pula os testes; com `-NoPackage` gera só o `.exe`.

## Publicar uma versão

1. Troque a versão em `cmd/axion/version.go`, no `-Version` padrão do `build/build.ps1` e no `CHANGELOG.md`.
2. Envie as mudanças. O CI (`.github/workflows/ci.yml`) roda os testes e gera o pacote a cada envio.
3. No GitHub, abra **Actions › Release › Run workflow** e informe a versão. O fluxo compila o pacote e cria um rascunho de versão com o `.zip` e o `SHA256SUMS.txt`.
4. Revise o rascunho em **Releases** e publique.

## Testes

```powershell
$env:GOFLAGS='-mod=mod'; $env:GOPROXY='off'; $env:GOSUMDB='off'; $env:CGO_ENABLED='0'
go test ./internal/...
```

Os testes rodam sobre o PC simulado e nunca escrevem na máquina real. A suíte inteira leva cerca de 1 minuto.
