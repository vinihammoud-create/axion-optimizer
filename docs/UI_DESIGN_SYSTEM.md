# Sistema visual da interface do Axion

Referência para quem mexe em `internal/web/static/` (`app.css`, `app.js`, `index.html`). Os valores vivem nos
tokens do começo do `app.css` (`:root`); este documento explica o uso de cada um. Mudou um token? Atualize a
tabela aqui.

A interface é escura por padrão ("dark-first") e não tem tema claro. Contrastes abaixo seguem a fórmula do WCAG 2.x
(AA: 4,5:1 para texto, 3:1 para borda de controle e ícone que carrega informação).

## 1. Cores da marca

Medidas por script sobre os pixels de `build/axion-mark.png` e `docs/img/axion-logo.jpg` (ver
`docs/audit/INTERFACE.md`, seção 5). As imagens da marca não se editam.

| Token | Valor | Uso |
|---|---|---|
| `--brand-blue` | `#38A4DC` | Azul do logo. Base de `--electric` e das translúcidas `--electric-*` (realce, foco, ícone ativo, barra de progresso) |
| `--brand-orange` | `#FC9655` | Laranja do logo. Base de `--accent` e `--accent-text` (jogo e desempenho, no máximo um destaque por tela) |
| `--silver-1` / `--silver-2` | `#E9EEF4` / `#9AA6B5` | Prata do nome e da abertura |

## 2. Ação e destaque (derivados da marca)

| Token | Valor | Uso | Contraste medido |
|---|---|---|---|
| `--primary` | `#1F78AE` | Fundo do botão principal, interruptor ligado, caixa de seleção. É o azul da marca escurecido, mesmo matiz | Texto branco: 4,8:1 |
| `--primary-pressed` | `#1A6896` | Botão principal pressionado | Texto branco: 6,1:1 |
| `--electric` | `var(--brand-blue)` | Contorno de foco, item ativo do menu, ícones de cartão, anel "Bom" | 5,4:1 a 7,1:1 sobre as superfícies |
| `--primary-text` | `#5DB6E3` | Link e texto de ação | 6,5:1 ou mais (inclusive sobre `--primary-soft`) |
| `--primary-soft` | `#18293D` | Fundo de pílula e filtro ativos | — |
| `--accent` | `var(--brand-orange)` | Fundo do botão de jogo (`.btn.game`) | Texto `--accent-ink` (`#1A0B00`): 8,8:1 |
| `--accent-text` | `var(--brand-orange)` | Texto e ícone de jogo | 6,8:1 sobre `--accent-soft` |
| `--accent-soft` | `#33241D` | Fundo de pílula e aviso de jogo | — |

Botão azul cheio com texto branco usa sempre `--primary`, nunca `--brand-blue` (branco sobre `#38A4DC` dá só
2,8:1).

## 3. Superfícies, bordas e texto

| Token | Valor | Uso |
|---|---|---|
| `--bg-0` | `#07090C` | Fundo da janela |
| `--bg-1` | `#0C0F14` | Caixas recuadas (detalhes, registro técnico) |
| `--surface-1` | `#12161D` | Cartão, linha de otimização, diálogo |
| `--surface-2` | `#181E27` | Campo, botão secundário, cartão em foco |
| `--surface-3` | `#202733` | Item ativo, aviso flutuante |
| `--border-subtle` / `--border` | `#1F2630` / `#2A3240` | Divisória e contorno de cartão (decorativos) |
| `--border-strong` | `#667285` | Contorno de controle: campo, botão, trilho do interruptor desligado. 3,1:1 ou mais sobre `--surface-1` a `--surface-3` |
| `--text-1` | `#F2F5F8` | Título e texto principal |
| `--text-2` | `#B6C0CC` | Texto de apoio; título de linha que não se aplica ou está bloqueada |
| `--text-3` | `#8D98A8` | Texto secundário (resumo, legenda). 4,6:1 ou mais em todas as superfícies e pílulas "soft" |
| `--text-disabled` | `#5E6878` | Só decoração (anel sem medição, ponto de estado). Nunca texto |

**Esmaecer sem `opacity`.** Linha que não se aplica, bloqueada ou já concluída não recebe opacidade no bloco
inteiro (o motivo escrito cairia abaixo de 4,5:1). Só o título passa de `--text-1` para `--text-2`.

## 4. Cores de estado

| Estado | Token | Onde |
|---|---|---|
| Tudo certo / Excelente (nota 90 a 100) | `--ok` `#34D27B` | Anel, barra, pílula, achado "Tudo certo" |
| Bom (75 a 89) | `--electric` (anel e barra), `--info` (pílula) | |
| Regular / Atenção (50 a 74) | `--warn` `#FFC247` | |
| Ruim / Importante (0 a 49) | `--bad` `#FF5C61` | |
| Sugestão | `--info` `#4FC3F7` | Achado do diagnóstico |
| **Não lido / informação insuficiente** | `--text-3` (anel e barra), `--text-2` (rótulo) | Classe `unread`. **Neutro, nunca verde** |
| Sem medição | `--text-disabled` | Anel com "—" |

A régua é uma só: `scoreClass` em `app.js` (90 / 75 / 50). Anel, barras por área e pílula "nota N" usam a mesma
função. Quando o relatório tem partes não lidas (`report.unread`), o anel fica no tom neutro (`s-unread`); cada
barra por área fica neutra (`.bar.unread`) e a pílula perde a cor quando aquela área depende de uma parte não lida
(`scores[].unread`, calculado em `internal/diag/unread_areas.go`): a nota leva em conta só o que foi lido e não
pode parecer "verde".

Dado não lido aparece escrito como "não lido" (função `tri`), nunca como "não", "sem" ou "desligado".

## 5. Tipografia

| Token | Valor | Uso |
|---|---|---|
| `--font` | Segoe UI Variable Text, Segoe UI | Texto (14 px / 22 px) |
| `--font-display` | Segoe UI Variable Display | Títulos (h1 a h4, título de linha e de achado) |
| `--mono` | Cascadia Mono, Consolas | Números (nota, contadores), valores do registro |

Tamanhos usados: 11-12 px (legenda, pílula), 13 px (texto de apoio), 14 px (corpo), 15 px (título de linha),
20 px (destaque do Início), 40 px (número do anel).

## 6. Espaçamento, raio e movimento

- Espaço em base 4: `--s-1` 4 px, `--s-2` 8, `--s-3` 12, `--s-4` 16, `--s-5` 20, `--s-6` 24, `--s-8` 32, `--s-10` 40,
  `--s-12` 48.
- Raio: `--r-xs` 4 px (pílula), `--r-sm` 6 (botão, campo), `--r-md` 10 (cartão, linha), `--r-lg` 14 (diálogo),
  `--r-full` (interruptor, filtro).
- Movimento: `--t-fast` 120 ms, `--t-med` 180 ms, `--t-slow` 240 ms; desligado com "Efeitos de animação" do
  Windows desligado (`prefers-reduced-motion`).

## 7. Anatomia da linha de otimização (`tweakRow`)

```
┌──────┬─────────────────────────────────────────────┬──────────────────────────────┐
│ [o ] │ Título da otimização                        │ [Recomendado] [risco seguro] │  1 estado  2 risco
│      │ Resumo de uma ou duas linhas (--text-3)     │ [Pede reiniciar o PC]        │  nota mais urgente
│      │ ┃ Para o seu PC: motivo em uma linha        │ Aplicado pelo Axion [Desfazer]│  3 origem  4 desfazer
│      │ › Detalhes e fontes                         │                              │
└──────┴─────────────────────────────────────────────┴──────────────────────────────┘
```

Os quatro sinais sempre visíveis, sem abrir nada:

1. **Estado** (pílula): Ativo, Recomendado, Opcional, Não indicado, Bloqueado neste PC, Não se aplica.
2. **Risco** (pílula, sempre, inclusive "risco seguro"): seguro (contorno), moderado (amarelo), avançado (vermelho).
3. **Origem**: "Aplicado pelo Axion" (há registro no Histórico), "Já estava assim no Windows" (está ativo sem
   registro do Axion) ou "Não aplicado". O estado que o servidor envia não diz se um valor não aplicado é o padrão
   do Windows, então a interface não escreve "padrão do Windows" nesse caso.
4. **Desfazer** (botão pequeno): só quando o Axion aplicou. Faz o mesmo pedido do botão Desfazer do Histórico
   (`POST /api/revert` com a entrada do registro, função `revertEntry`) e restaura o valor anterior guardado.

O interruptor à esquerda continua aplicando e desfazendo; o botão Desfazer é o caminho escrito para quem não
reconhece o interruptor como "desfazer". Notas que dependem do estado (desfazer pendente, desfeito depois,
parcial, não está valendo, vale após reiniciar, pede reiniciar) aparecem uma por vez, a mais urgente; as demais
ficam em "Detalhes e fontes".

## 8. Perfil de uso e navegação

- Perfis que jogam (`gaming: true`, de `/api/profiles`): o menu traz "Jogos e medição" logo após o Diagnóstico;
  o ciclo do Início termina em "Medir no seu jogo"; a paleta (Ctrl+K) oferece "Aplicar perfil CS2" e "Medir FPS".
- Perfis que não jogam (ex.: Trabalho e escritório): o grupo "Jogos e medição" vai para o fim do menu (nenhuma
  página some); o terceiro passo do ciclo é "Avaliação completa"; a paleta não oferece o perfil CS2 nem a medição
  de FPS.
- O estado (`/api/state`) é relido a cada 15 s só com a janela visível; oculta, a leitura para e acontece uma vez
  quando a janela volta.

### Os dois níveis: Simples e Avançado

```
Início                       ( Simples | Avançado )  [Buscar… Ctrl K]  (● Analisado agora)  [Reanalisar]
```

- **Seletor:** sempre na barra superior, antes da busca. É um grupo de rádio (`role="radiogroup"`, cada opção
  `role="radio"` com `aria-checked`): o Tab chega na opção marcada; setas, Home e End trocam o nível, e o foco
  fica no seletor. A opção marcada usa `--primary-soft`, borda `--electric-40` e texto `--primary-text`.
- **Gravação:** `settings.mode` (`simples` ou `avancado`, `app.ModeSimple`/`app.ModeAdvanced`). O booleano
  antigo `simple` continua gravado igual ao modo, para uma versão anterior ler o mesmo nível. Sem
  `settings.json` (instalação nova) o nível é Simples; um arquivo já gravado sem `mode` vale pelo `simple`.
- **Simples:** sem menu lateral nem paleta. Mostra uma tela (nota, até 3 achados, "Aplicar recomendadas",
  "Desfazer mudanças") e o Histórico (`SIMPLE_PAGES`), com "Voltar ao início". "Aplicar recomendadas" usa o
  plano `recomendado` do servidor, que só aceita itens sem risco (`Tweak.Recommendable`); a interface não
  cria regra própria. Nos achados, só ficam as ações de página, de link e as que aplicam itens desse plano.
  Qualquer outra página troca para o Avançado e avisa.
- **Avançado:** a interface completa (todas as páginas, todas as otimizações, valores do registro e dados lidos).

## 9. Rascunho das telas

### Início

```
Início                                                        [Trocar perfil] [Exportar]
Última análise há 2 min · perfil Trabalho e escritório
[CPU] [GPU] [RAM] [Monitor] [Windows]                      ← faixa de chips do hardware
┌ SAÚDE DO PC PARA O TRABALHO ──────┐ ┌ PRÓXIMO PASSO ───────────────────────────────┐
│ (anel 44)  Precisa de atenção      │ │ 14 melhorias recomendadas para o seu PC      │
│            resumo · [Ver o que pesa]│ │ [Aplicar as 14 recomendadas] [Ver cada uma]  │
└────────────────────────────────────┘ └──────────────────────────────────────────────┘
(1 Analisar ✓)  (2 Aplicar recomendadas · 14 pendentes)  (3 Avaliação completa | Medir no seu jogo)
[2 Ativas] [14 Recomendadas] [30 Opcionais] [3 Não indicadas]
┌ Recomendadas para o seu PC ─────────┐ ┌ Precisa de atenção ─┐
│ linhas de otimização (seção 7)       │ │ até 4 achados        │
│                                      │ ├ Notas por área ──────┤
│                                      │ │ barras (seção 4)     │
└──────────────────────────────────────┘ └──────────────────────┘
Seus jogos · Outras formas de otimizar · Seu PC em detalhes (recolhido)
```

### Otimizar

```
[Otimizações] [Prova no seu PC] [Histórico]                 ← abas do grupo
Otimizações                                     [Aplicar recomendadas (14)]
(Recomendadas 14) (Ativas 2) (Opcionais 30) (Não indicadas 3) (Não se aplicam 38) (Todas 49)
┌ Buscar ─────────────┐ ┌ Todas as categorias · 2 de 49 ativas ──────────────────┐
│ Todas as categorias  │ │ CORREÇÕES                                               │
│ Correções         5  │ │ linha de otimização (seção 7)                           │
│ Sistema e jogo    1  │ │ linha de otimização                                     │
│ …                    │ │ …                                                       │
└──────────────────────┘ └─────────────────────────────────────────────────────────┘
```

## 10. Uma frase na tela, o resto no "?"

Regra: cabeçalho de página, cartão ou aviso mostra **uma frase** (de preferência até 120 caracteres) com o que
importa para decidir. O resto da explicação fica atrás de um botão "?" e só aparece quando o usuário pede. Nada é
apagado: o texto muda de lugar, com o mesmo sentido.

```
Desempenho (FPS)
Mede FPS, 1% e 0,1% low, engasgos e o que limitou cada quadro, e compara antes × depois. (?)
┃ O que limita cada quadro é a CPU, a GPU ou um limite de FPS. A medição usa o PresentMon…   ← aberto
```

Componente único em `app.js` (seção "Components"):

- `helpText(frase, resto, { cls, style, label })`: a frase, o "?" logo depois dela e o painel com o resto (texto,
  nó ou lista deles, um parágrafo cada). `cls` e `style` vão no invólucro, então a frase mantém a aparência do
  parágrafo que substitui (`page-desc` no cabeçalho de página, `sub`, `small muted`…).
- `helpLead(texto)`: para parágrafo que vem pronto do servidor (os textos "Como funciona", `honest`, `privacy`).
  O texto do Go não muda; só a exibição se divide: a primeira frase fica na tela e o resto vai para o "?".
  Frase longa demais é cortada onde anuncia uma lista (": "), salvo `{ wholeSentence: true }`.
- Por baixo, `help(resto, rótulo)` devolve `{ btn, panel }` para quem precisar montar outro arranjo.

Comportamento e acessibilidade:

- O "?" é um `<button>` de verdade (24 × 24 px, `--surface-2`, borda `--border-strong`), com `aria-expanded`,
  `aria-controls` apontando para o painel e `aria-label` "Mais detalhes: <começo da frase>". Enter ou Espaço
  abre e fecha; Esc (no botão ou no painel) fecha e devolve o foco ao botão, sem fechar o diálogo em volta.
- O painel abre **no lugar**, logo abaixo da frase (`.help-panel`: `--bg-1`, borda `--border` com filete
  `--electric` à esquerda, texto `--text-2` 13 px). Não é balão flutuante: nenhum cartão, tabela ou diálogo o
  corta, e o leitor de tela o encontra logo depois do botão.
- Um painel aberto continua aberto quando a página é redesenhada (mesma chave, tirada do texto) e o foco volta
  ao mesmo botão (`data-focus`).

Não usar o "?" em: evidência e explicação de achado que já ficam num `<details>`, detalhe de cada otimização,
confirmações de aplicar, desfazer e instalar (o usuário precisa ler antes de confirmar), mensagens de erro e estados
vazios. Aviso que evita expectativa errada fica na frase visível (ex.: "não aumenta o FPS" no reparo do Windows).
