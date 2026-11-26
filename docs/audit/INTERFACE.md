# Auditoria da interface: texto, redundância, modo simples, cores, contraste e peso

Auditoria somente leitura (issue nº 26). Base: `main` no commit `246119d`. Nenhum código foi alterado.
As referências `arquivo:linha` valem para esse commit. Sem pasta, `app.js` e `app.css` são os de
`internal/web/static/`.

**Como foi feito.** Leitura do `app.js`, do `app.css` e do `index.html`; consulta ao Go só onde a interface
depende dele (`internal/diag`, `internal/facts`, `internal/tweaks`, `internal/app`). As cores da marca foram
medidas por script sobre os pixels das imagens e os contrastes calculados pela fórmula WCAG 2.x a partir de
`:root`. A interface **não foi aberta** num navegador nem no WebView2: o que depende de renderização real
está em "Não verificado".

Confiança: **alta** = li o código e o comportamento decorre dele; **média** = li o código, mas o efeito
depende de dados ou de renderização que não rodei; **baixa** = inferência.

## Resumo

1. **O sinal "informação insuficiente" não existe.** As regras do diagnóstico se calam quando a leitura
   falhou (`facts.Unread`), mas a nota continua alta e o resumo diz "Nenhum problema importante
   encontrado". A interface nunca lê `unread` nem `tpmRead`, e mostra "não" ou "sem TPM" para dado não
   lido (seção 3.3). É o achado mais grave: contraria a regra "dado não lido não é normal".
2. **O Modo simples já é persistente** (`settings.simple`), mas tem botões que não fazem nada e nenhum
   caminho para reverter (seção 3.2).
3. **Um seletor Simples/Avançado por risco é viável sem reescrever a lógica:** `NAV` é uma tabela, o risco
   já vem em `/api/tweaks` e o servidor já limita o 1 clique a risco seguro (seção 3.4).
4. **Texto longo na superfície:** 55 textos de uma linha com 150 caracteres ou mais, em 34 funções, cerca de
   11 mil caracteres. Cinco passam de 325, e o pior (`perfCompareCard`) tem 502 (seção 1).
5. **Redundância:** a contagem de "recomendadas" aparece em 6 lugares; notas por área, achados e hardware
   repetem em 2 ou 3 páginas (seção 2).
6. **Marca:** `--primary` (#1F6FEB) e `--accent` (#FF7A1A) não existem na marca; o azul medido é ~#38A4DC e o
   laranja ~#FC9655 (seção 5).
7. **Contraste:** o texto passa em AA nas superfícies principais; linhas esmaecidas por `opacity` e bordas
   de controles ficam abaixo do mínimo (seção 6).
8. **Peso:** 340 KB de JS sem minificar, sem custo relevante em `127.0.0.1`. O ponto de atenção é a página
   refeita inteira a cada tecla na busca e as 88+ linhas de otimizações montadas de uma vez (seção 7).

---

## 1. Texto explicativo longo na superfície

Contagem: literais `'...'` de uma linha com 150 caracteres ou mais (ícones SVG excluídos). Textos que o
servidor envia (`cat.description`, `plan.note`, `scoreNote`, `p.honest`) não entram. Regra proposta: na
superfície ficam o título e **uma** frase de até ~90 caracteres; o resto vai atrás de um botão "?" (ver 3.4).

| Onde (função: linha) | O que está na superfície | Sugestão | Conf. |
|---|---|---|---|
| `pagePainel` `app.js:1110` | Parágrafo "Só itens com evidência… sem risco e que podem ser desfeitos" no cartão "Próximo passo" | "?" do cartão; a linha `1109` já diz "tudo pode ser desfeito" | alta |
| `pagePainel` `app.js:1117` | Parágrafo do estado "Tudo certo" (3 orações) | Uma frase; o resto no "?" | alta |
| `pageOpt` `app.js:1463` | Cabeçalho de 138 caracteres, mais a `headDesc` (`1445`) logo abaixo | Manter um só; o outro no "?" | alta |
| `pageProva` `app.js:2155` | Cabeçalho de 154 caracteres sobre como a prova decide | Uma frase + "?" | alta |
| `pagePerf` `app.js:2268`, `2302`, `2304` | Cabeçalho (185) e dois parágrafos de definição (176 e 340): FPS, 1% low, "limitado pela GPU" | Cada definição vira "?" ao lado da métrica | alta |
| `perfAdvice` `app.js:2025-2036` | Até 5 conselhos de 164 a 335 caracteres empilhados após a medição | Mostrar só o do limitador; o resto em "Outras observações" | média |
| `perfCompareCard` `app.js:2110` | **502 caracteres** sobre média, desvio e teste t de Welch | "?" no cabeçalho da tabela; na tela, só o veredito | alta |
| `pageApps` `app.js:2701`, `2704`, `2713` | Três blocos seguidos (185, 213 e 294) antes da lista | Um bloco curto; o resto no "?" | alta |
| `appsInstall` `app.js:2624` | 345 caracteres no diálogo de instalação | Linha curta + "?" | média |
| `pagePartidas` `app.js:2965` | **328 caracteres** de "Como funciona" no topo | "?" do cabeçalho | alta |
| Página Rede: `netTargetsCard` `3116`, `netBloatCard` `3144`, `netRelaysCard` `3154`, `netPathCard` `3169`, `netCS2Card` `3194` | Um parágrafo de 159 a 220 caracteres no topo de cada cartão | "?" por cartão | alta |
| `pageSaude` `app.js:3632`; `repairCard` `app.js:3619` | Cabeçalho (210) e aviso do reparo (261) | "?". Manter visível que o reparo "não aumenta o FPS": evita expectativa errada | média |
| Página Drivers: `gpuDriverCard` `1782`, `driverOffersCard` `1855`, `oldDriverPackagesCard` `1866`/`1884`, `boardCard` `1910` | Avisos de 164 a 227 caracteres | Só o aviso de risco ("o Axion não consegue desfazer isto", `1884`) fica visível | média |
| `activeHoursCard` `app.js:1393`, `1406` | Dois parágrafos de ~190 caracteres | "?" | média |
| `scanView` `app.js:3820` | Parágrafo de 183 caracteres na tela inicial | Uma frase | baixa |
| `pageSobre` `app.js:2865-2881` | 9 itens de 151 a 251 caracteres | É a página de metodologia: texto longo é esperado. Sem mudança | alta |

**Já está certo:** "Como a nota é calculada" fica recolhido (`app.js:1170`), o detalhe de cada otimização fica
em `<details>` (`1039`, com `noDetail` no Início) e o hardware fica em "Seu PC em detalhes" (`1195`).
A linha de otimização (`tweakRow`, `1003`) é densa (título, resumo, "Para o seu PC:" em 1 linha, pílula de
estado), mas útil; o excesso está mais nos cabeçalhos do que nas linhas.

## 2. Redundância entre abas

| Informação | Onde aparece | Achado | Conf. |
|---|---|---|---|
| Quantas otimizações estão recomendadas (`nRec`) | Contador do menu (`859`), título do cartão (`1108`), título da lista (`1119`), degrau do ciclo (`1127`), estatística "Recomendadas" (`1175`), botão do cabeçalho de Otimizações (`1464`) | Seis lugares. Deixar o cartão "Próximo passo" e o contador do menu | alta |
| Notas por área | "Notas por área" no Início (`1186`), barras por área no Diagnóstico (`1285-1286`), três pilares na Avaliação (`1262-1264`) | Início e Diagnóstico desenham a mesma barra pela mesma regra. No Início, só a nota geral e o link | alta |
| Achados | "Precisa de atenção" no Início (`1183`), Diagnóstico (`1284`), `pillarCard` da Avaliação (`1232`) | O pilar "Configuração" é o Diagnóstico de novo (`1262`): já tem link, mas repete os cartões | alta |
| Hardware | Faixa de chips (`1139`), "Seu PC em detalhes" (`1195-1207`), "Dados coletados deste PC" (`1331-1354`) | Três apresentações do mesmo `/api/facts`. Faixa e "detalhes" podem virar uma | alta |
| Segurança/anti-cheat | Chip (`1144`), tile (`1204-1205`), bloco do Diagnóstico (`1345-1351`), achados `sec.*` | Quatro lugares; dois deles afirmam "sem TPM" sem checar a leitura (3.3) | alta |
| "Pode ser desfeito" | Boas-vindas (`3880`), cartão "Próximo passo" (`1109`), cabeçalho de Otimizações (`1463`), cabeçalho do Histórico (`2824`), diálogo (`785`) | Cinco frases dizendo o mesmo. Trocar por um sinal fixo por item (3.4) | média |
| Modo simples | Botão no Início (`1160`), botão "Modo completo" (`805`), paleta (`3691`) | Três entradas, nenhuma na barra superior nem no menu | alta |
| Perfil CS2 | Degrau "Perfil Counter-Strike 2" (`1193`), cartão do jogo (`1155`), página `cs2` | Atalho duplo; não é erro, só ruído | baixa |

## 3. Modo simples e viabilidade do seletor Simples/Avançado

### 3.1 Como funciona hoje

- **Estado:** `settings.simple` (bool) em `internal/app/app.go:48`, gravado por `POST /api/settings`
  (`internal/app/server.go:566,583`). É **persistente** entre sessões.
- **Cliente:** `isSimple()` (`app.js:723`); `setSimple(on)` grava e chama `rerender(true)` (`764`).
- **Onde se decide o que some** (não há ponto único; são 6):
  1. `render()` troca a página por `pageSimple` e ignora a rota (`3767-3784`);
  2. o CSS esconde a barra lateral: `.app.simple .sidebar` (`app.css:201`);
  3. a barra superior troca o título (`892`) e tira "Buscar ou ir para" (`897`);
  4. a paleta de comandos fica desativada (`3678`);
  5. o banner de relatório de partida não aparece (`910`);
  6. `pageSimple` mostra só a nota, os 3 achados mais pesados e um botão (`797-819`).
- Não há filtro por risco: o Modo simples não esconde ajustes, troca a página inteira por outra. Os ajustes
  só aparecem no diálogo `solveSafe` (`772-795`), com título e resumo, sem risco nem evidência por item.

### 3.2 Problemas encontrados

| Achado | Onde | Conf. | Como corrigir |
|---|---|---|---|
| **Botões sem efeito.** `findingCard` monta as ações do servidor com `actionButton`; a ação `page` chama `go()` (`943`, `240`), que só muda o `hash`, e `render()` volta a `pageSimple`. O servidor emite 32 ações `Kind: "page"` em `internal/diag` | `app.js:817`, `943` | alta | No modo simples, não desenhar ações `page`, ou sair do modo com confirmação ao clicar |
| **Banner "Ver" sem efeito:** "otimização foi desfeita" chama `go('diagnostico')` e aparece também no modo simples | `app.js:907` | alta | Esconder o botão no modo simples, como já se faz com o banner de partida (`910`) |
| **Sem caminho para reverter:** o diálogo manda ao "Histórico (modo completo)", inalcançável sem trocar de modo | `app.js:785` | alta | "Desfazer o último lote" no próprio modo simples, ou link que troca de modo e abre o Histórico |
| **Quem entra não vê como sair:** só o cabeçalho tem "Modo completo"; a barra superior mostra "Axion · modo simples" sem controle | `app.js:805`, `892-893` | média | O seletor na barra superior (3.4) |

### 3.3 O sinal "informação insuficiente" não existe

- `diag.Score` (`internal/diag/diag.go:226-285`) parte de 100 e só desconta por achados. Quando a leitura
  falhou, a regra emite **nada** (`diag.go:732`, `965`, `1024`; o teste `TestUnreadStepsDoNotCreateFindings`,
  `diag_test.go:634`, exige isso). O resultado é "Excelente" e "Nenhum problema importante encontrado"
  (`diag.go:266`, `277`), com anel verde. `Report` não tem campo de leitura incompleta.
- `Facts.Unread` chega à interface em `/api/facts` (`facts.go:798`), mas o `app.js` **não lê `unread`**
  (`grep -n "\.unread"` só acha o status de saúde, `3657`). `tpmRead` (`facts.go:360`) também nunca é lido.
  Os avisos da coleta (`f.errors`) ficam no fim de um `<details>` fechado (`1354`).
- Efeito visível: `app.js:1204` mostra "sem TPM" e "sem Secure Boot" sem olhar `tpmRead`; `1205` mostra
  "Nenhum anti-cheat de kernel encontrado" para lista vazia, sem olhar `unread`; `1345-1351` mostra
  "VBS em execução: não" e "TPM: não encontrado" com a mesma falha. Conf. **alta** no código; o efeito
  depende de uma coleta realmente falhar, que só reproduzi lendo o código.
- **Correção:** (a) o servidor devolve em `Report` um campo como `incomplete`, com os nomes de `Unread`, e a
  nota nesse caso não é "Excelente": estado próprio, cor neutra, texto "informação insuficiente"; (b) a
  interface mostra esse estado no anel e no resumo, e troca "não"/"sem TPM" por "não foi possível ler" nos
  blocos acima. `pageSaude` (`3567-3587`) e `thermalBlock` (`1314-1321`) **já fazem certo** e servem de modelo.

### 3.4 Viabilidade do seletor persistente Simples/Avançado

**É viável sem reescrever a lógica.** O que já existe:

- `NAV` (`app.js:828-836`) é dado puro: acrescentar `level` a cada entrada e filtrar em `renderSidebar`
  (`852`) e `subnav` (`3751`). `ROUTES` não muda.
- O risco já chega por item (`t.risk`: seguro, moderado ou avançado) e o servidor já barra o resto do 1 clique:
  `Tweak.Recommendable()` exige `RiskSafe` e `OneClick` (`internal/tweaks/model.go:175`). No catálogo há 75
  itens `RiskSafe`, 11 `RiskModerate` e 2 `RiskAdvanced` (contagem por `grep` em `internal/catalog`).
- O filtro por risco entra num ponto: `optMatch` (`app.js:1363`) e a lista de `pageOpt`.
- `settings.simple` já persiste; basta um valor de 2 estados, mantendo `omitempty` para configurações antigas.

**Decisão do dono antes de implementar** (muda o que o usuário vê): hoje o Modo simples é uma tela só e
remove o menu. O seletor o transformaria em "o app com menos entradas"?

**Riscos:**

- Os 6 pontos de 3.1 teriam de ler a mesma função (`levelOf(page)`), senão o estado diverge.
- `tweakBucket` (`982-992`) já reescreve no cliente a regra do servidor (o *fallback* `lvl === 1 &&
  t.oneClick`). Um filtro por risco no cliente seria a terceira cópia; melhor o servidor devolver o nível.

Os quatro sinais do plano, e o que cada tela mostra hoje:

| Sinal | Hoje | Lacuna |
|---|---|---|
| Estado do diagnóstico (inclusive "insuficiente") | Anel e `SEV`: Importante, Atenção, Sugestão, Tudo certo (`app.js:359-364`) | Falta o 5º estado (3.3) |
| Risco do ajuste | `riskBadge` só se `risk !== 'seguro'` e o item não está ativo (`1022`); "seguro" só aparece em "Confiança" dentro do `<details>` (`1029`); no plano só se não for seguro (`611`) | O risco "seguro" nunca é dito na linha: é dito por ausência |
| Estado atual (aplicado ou padrão) | Pílula `Ativo`, `Já ativo no Windows`, `Recomendado`… (`BUCKET`, `994-1001`; `1012`) | Não há "padrão do Windows"; `Ativo` mistura "aplicado pelo Axion" com os demais casos |
| Caminho para reverter | Só o `aria-label` do interruptor: "Desfazer: …" (`1010`) | Sem texto visível; no modo simples não existe (3.2) |

## 4. Acoplamento entre diagnóstico e HTML

O `app.js` (3.924 linhas) mistura apresentação e regra. Os pontos de maior acoplamento:

| Regra | No JS | Cópia no Go | Conf. |
|---|---|---|---|
| Limiares da nota 90/75/50 | `scoreClass` `app.js:347` | `diag.Score` `diag.go:266-274` | alta |
| **Limiar 60 nas barras por área**, diferente do 75/50 acima | `app.js:1188`, `1286` | não existe no Go | alta |
| Fator de engasgo 2,5 (`StutterFactorJS`) e o texto "2,5 × média" | `app.js:2012`, `2000` | `perf.StutterFactor`, `perf/stats.go:21` | alta |
| Regra de "recomendada" (`t.recommended` e o *fallback* por nível e `oneClick`) | `tweakBucket` `982-992` | `Tweak.Recommendable` `model.go:175` | alta |
| Conselhos por limitador, `stutterPct >= 1`, `syncInterval >= 1` | `perfAdvice` `2025-2036` | `perf.Summary` só entrega números | média |
| Quais achados contam (dispensado, fora do perfil) | `activeFindings` `793`, repetido em `1233` e `1272-1274` | `diag.Dismissible` `diag.go:180` | média |
| O que está "pendente" no modo simples | usa `st.pendingRecommended` (servidor) | — | **certo** |

Os dois limiares dão cores diferentes à mesma nota: de 75 a 89 a barra por área é amarela (≥ 60) e o anel
é azul, "Bom" (`scoreClass`); de 50 a 59 a barra é vermelha e o anel é amarelo, "Regular". Conf. alta de que
são dois limiares; baixa de que o usuário note.
**Correção:** o servidor envia a classe (`ok`/`info`/`warn`/`bad`) junto de cada nota; o nível de cada ajuste
e o fator de engasgo vêm de `/api/tweaks` e `/api/perf`. Isso também resolve o filtro de 3.4.

## 5. Cores da marca contra os tokens de `:root`

**Medido** por script sobre os pixels (azul: matiz 180°-223°; laranja: 11°-43°; ambos com saturação > 55% e
valor > 60%; prata: saturação < 15% e valor > 60%). Mediana de cada grupo:

| Cor | `build/axion-mark.png` | `docs/img/axion-logo.jpg` | Matiz / sat. / valor |
|---|---|---|---|
| Azul da marca | #38A4DC | #399ED0 | 199-200° / 73-75% / 82-86% |
| Laranja da marca | #FC9655 | #F58F52 | 22-23° / 66-67% / 96-99% |
| Prata da marca | #CEDCE7 | #CAD6DE | 204-206° / 9-11% / 87-91% |
| Fundo (média dos pixels escuros) | #121A27 | #080C15 | 217-222° / 54-62% / 8-15% |

**Divergências** (a imagem tem brilho e degradê; conf. **média**):

| Token (`app.css`) | Valor | Marca mais próxima | Divergência |
|---|---|---|---|
| `--electric` (`:33`) | #3BA0FF (209°, 77%, 100%) | Azul ~#38A4DC | Matiz ~9° mais violeta; valor 14-18 pontos mais alto |
| `--primary` (`:31`) | #1F6FEB (216°, 87%, 92%) | Azul ~#38A4DC | **Fora da marca:** 16° de matiz e bem mais escuro. É a cor do botão e do interruptor ligado |
| `--info` | #4FC3F7 (199°, 68%, 97%) | Azul ~#38A4DC | O mais próximo do azul do logo, mas é token semântico (informação) |
| `--accent` (`:38`) | #FF7A1A (25°, 90%, 100%) | Laranja ~#FC9655 | Saturação 23 pontos maior: o laranja do app é puro, o do logo é pastel |
| `--silver-1` / `--silver-2` | #E9EEF4 / #9AA6B5 | Prata ~#CAD6DE | A faixa do meio do prata (#CAD6DE a #CEDCE7) não existe nos tokens |
| `--bg-0`, `--surface-1` | #07090C / #12161D (sat. 38-42%) | Fundo #080C15 e #121A27 (sat. 54-62%) | Superfícies mais cinzas que o azul-petróleo do logo |
| `favicon.svg` | "A" desenhado à mão em #3BA0FF e #FF7A1A | Marca aprovada (`logo-48.png`) | **Não é a marca aprovada.** `index.html:9-10` declara os dois ícones. Impacto baixo: o WebView2 não mostra aba |

`logo-256.png` é idêntico a `docs/img/axion-mark.png` (`cmp`). Os tokens `--ok`, `--warn`, `--bad` e
`--chart-*` não são da marca por definição e ficam fora.
**Como corrigir (só se o dono quiser fidelidade):** acrescentar `--brand-blue` (~#38A4DC) e `--brand-orange`
(~#FC9655) e usá-los na marca, no anel e nos destaques, mantendo `--primary` e `--accent` onde o contraste
exige. Não mexer nas imagens da marca.

## 6. Contraste (WCAG 2.x: AA = 4,5:1 para texto, 3:1 para ícone e borda de controle)

Texto sobre as cinco superfícies (`--bg-0` a `--surface-3`), menor razão de cada token:

| Token | Menor razão | Resultado |
|---|---|---|
| `--text-1` / `--text-2` | 13,7 / 8,2 | passa |
| `--text-3` | **4,95** (em `--surface-3`) | passa, com folga de 0,45 |
| `--primary-text`, `--accent-text`, `--bad-on-soft`, `--ok`, `--warn`, `--info` | 6,0 a 12,4 | passa |
| `--bad` | 4,97 | passa |
| `--text-disabled` | 2,66 | **não passa**, mas só colore o anel `s-none` (`app.css:426`) e o ponto `.dot.w` (`889`); não é texto |
| `--primary` como texto | 3,24 a 4,30 | **não passa**, mas não é usado como texto (só fundo: `334`, `375`) |

Sobre as superfícies "soft", os pares de `.badge.*` e `.callout` passam (5,0 a 8,4). Botões: branco sobre
`--primary` 4,63 (passa por pouco), sobre `--danger-fill` 4,79; tinta sobre `--accent` 7,37. **Exceção
marginal:** `--text-3` sobre `--warn-soft` dá 4,45; ocorre se um `.muted` ficar dentro de `.callout.warn` ou
do banner (`232-238`); não achei um uso exato (conf. baixa).

**Falhas encontradas**

| Achado | Onde | Medida | Conf. | Como corrigir |
|---|---|---|---|---|
| Linhas "Não se aplica" esmaecidas com `opacity: .55` no cartão inteiro: o resumo (`.ts`, `--text-3`) cai a ~2,7:1 e o motivo (`.tw`, `--text-2`) a ~3,8:1. É o texto que explica **por que não se aplica** | `app.css:638`, `624` | ~2,7 / ~3,8 | média (valor aproximado, ver "Não verificado") | Tirar a opacidade do cartão; esmaecer só o título |
| Linhas "Bloqueado neste PC" com `opacity: .7` | `app.css:636` | `--text-3` ~3,6 | média | Idem |
| Borda dos campos (`--border-strong` #3A4556) | `app.css:392-393` | 1,73 sobre `--surface-2` (pede 3:1, SC 1.4.11) | média | Borda mais clara, ≥ ~#6B7788 |
| Interruptor desligado: trilho `--border-strong` | `app.css:373` | 1,87 sobre `--surface-1` | média (a posição do botão também indica o estado) | Trilho mais claro; é o sinal "aplicado ou padrão" |
| `.drv-offer.done` (`842`) e `.lever.done` (`913`), opacidade .72 e .75 | `app.css` | `--text-3` ~3,7 e ~3,9 | baixa | Mesma correção |
| Texto de 10 e 11 px em `--text-3` | `.brand-sub` `170`, `.nav-sep` `173`, `.eyebrow` `135` | passa em contraste, mas é pequeno | baixa | Subir para 12 px |

Não depende só da cor: `badge` leva texto, `findingCard` leva rótulo e o anel leva número. O foco de teclado
tem contorno de 2 px em `--electric` (`app.css:122`), razão 5,5 a 7,3: passa.

## 7. Peso e custo de renderização

| Arquivo | Bytes | Linhas | Obs. |
|---|---|---|---|
| `app.js` | 340.562 (≈ 94 KB com gzip -9) | 3.924 | Sem minificar; 53 linhas com mais de 300 caracteres (a maior, 784, na `1335`) |
| `app.css` | 64.649 (≈ 12,8 KB com gzip) | 1.034 | |
| `index.html` | 883 | 26 | |
| `logo-256.png` | 122.220 | | Usada a 96-120 px (`app.js:2856`, `3818`, `3876`); 256 px serve telas 2×. Marca: não tocar |
| `logo-48.png` | 5.800 | | |

- O servidor responde **sem cache** e sem compressão (`server.go:651`, `FileServer` direto): o `.js` inteiro
  é relido a cada carga. Em `127.0.0.1` o custo é desprezível (conf. média; não medi no WebView2).
- **`innerHTML`:** 4 usos (`92` ícones estáticos, `353` anel, `2009` e `3077` gráficos). Os dois gráficos
  montam o SVG só com números, sem texto do usuário: sem risco de injeção. O resto usa o construtor `h()`
  (com `textContent`), o que é certo.
- **Página refeita a cada tecla.** A busca de Otimizações (`1420`, 220 ms) e a de Programas (`2736`, 180 ms)
  chamam `rerender()`, que refaz a página inteira. O foco é preservado por `data-focus`. Em "Todas as
  categorias" são 88 ou mais linhas, e `tweakRow` monta o `dl` de detalhes (operações, fontes) mesmo com o
  `<details>` fechado (`1027-1040`). Conf. média (não medi o tempo). Correção: montar o conteúdo no evento
  `toggle` do `<details>`.
- **Listas sem limite:** `pageProcesses` (`2487`) e `pagePrograms` (`2721`) desenham tudo, sem paginação.
  Conf. baixa: sem dado de um PC real.
- **`setInterval` de 15 s** (`app.js:3920`): `refreshState()` refaz barra lateral, barra superior e banners
  (`renderChrome`, `920`), mesmo com a janela oculta (não há `visibilitychange`). Custo baixo; conf. alta de
  que acontece, baixa de que importe. Durante a gravação de partida há ainda um `setTimeout` de 5 s (`207`).
- CSS: `backdrop-filter` em 1 lugar (`app.css:707`, fundo de diálogo) e um bloco de
  `prefers-reduced-motion` (`1031`). Sem custo notável.

## 8. O que está certo

- A interface é HTML/CSS/JS puro, sem fonte nem script externo, e a CSP (`server.go`, constante `csp`) a
  confirma.
- `pageSaude` e `thermalBlock` tratam "não lido" como estado próprio (`3567`, `1314-1321`).
- Os tokens de texto passam em AA nas cinco superfícies (seção 6) e o foco é visível.
- Detalhes longos já ficam em `<details>` na linha de otimização e na nota do Início.
- O Modo simples usa o número do servidor (`pendingRecommended`) em vez de recalcular "recomendadas".

## Não verificado

- **Renderização real:** não abri a interface no WebView2 nem em navegador. Tempos de pintura, quebra de
  linha e altura das páginas não foram medidos.
- **Contraste com `opacity`:** os valores ~2,7 e ~3,8 são aproximados (a opacidade mistura o fundo do cartão
  com o da página). Confirmar com o seletor de cores do navegador.
- **Cor da marca:** as medianas vêm de filtro HSV sobre imagens com brilho e degradê; a "cor oficial" pode
  ser outra. O repositório não traz especificação de marca para conferir.
- **Falha de leitura de verdade:** a seção 3.3 vem do código e de um teste; não reproduzi uma coleta falha de
  ponta a ponta.
- **Contagem de ajustes por risco (75/11/2):** `grep` em `internal/catalog`; itens montados por funções
  auxiliares podem estar fora. O total real vem de `/api/tweaks`, que não consultei.
- **Páginas Periféricos, Drivers, Latência do sistema, Live (OBS) e Atraso de entrada:** li os textos de
  cabeçalho e os parágrafos; não li a lógica de cada uma.
- **Fontes da internet:** nenhuma foi aberta. As razões de contraste seguem a fórmula do WCAG 2.x e os
  limiares (4,5:1 no SC 1.4.3; 3:1 no SC 1.4.11) são citados de memória, sem conferir o texto da norma.
