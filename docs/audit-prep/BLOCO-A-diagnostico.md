# Bloco A: o diagnóstico é honesto?

Cole este texto inteiro numa conversa nova do OpenHands, com o repositório `vinihammoud-create/axion-optimizer` no branch `audit/v0.9.1-baseline`.

---

Você é um auditor externo de código. Seu trabalho é **somente leitura**: não edite arquivos, não faça commit, push nem pull request. A única saída é um relatório em Markdown na sua resposta final.

Comece lendo `docs/audit-prep/REPO_MAP.md`. Ele lista o que já foi conferido e o que as ferramentas automáticas já acharam; não repita esses itens.

## A pergunta

O Axion promete um diagnóstico baseado em evidência. A falha mais grave que ele pode ter é **tratar dado ausente como dado bom**: um sensor que não respondeu, uma leitura que falhou ou uma medição que não foi feita aparecer como "está tudo normal", ou somar pontos na nota.

Um resultado de diagnóstico correto cai em um de quatro estados:

1. Problema confirmado
2. Possível problema
3. Informação insuficiente
4. Condição normal

O código tem quatro severidades (`internal/diag/diag.go:26-29`: `critical`, `warning`, `info`, `good`), e nenhuma delas é "informação insuficiente". Esse estado aparece espalhado em flags (`Unread` em `internal/facts/facts.go:798`, `TPMRead` em `facts.go:360`, `ZonesRead` e `LimitRead` em `internal/facts/thermal.go`) e num mapeamento da interface (`internal/web/static/app.js`, perto da linha 1051, onde `unknown` vira `info` ou `outline`).

## O que ler

- `internal/diag/` inteiro, começando por `rules` em `diag.go:598`
- `internal/facts/facts.go` e `internal/facts/thermal.go`
- `internal/collect/collect.go`, `health.go`, `thermal.go`
- `internal/perf/` inteiro e `internal/app/perf.go`, `perfsensors.go`, `evaluate.go`
- Em `internal/web/static/app.js`, só as funções que pintam severidade e nota

Não leia `third_party/`, `internal/catalog/` nem `internal/tweaks/` (são do Bloco B).

## Hipóteses a verificar

Cada uma é uma suspeita, não um fato. Confirme ou descarte com `arquivo:linha`.

- **A1. Regra sem trava de leitura.** Para cada regra em `rules`: quando o fato que ela usa não foi lido, ela fica calada, emite um aviso falso ou conclui "bom"? Liste as regras em que valor zero, vazio ou ausente é lido como normal.
- **A2. Nota inflada por falta de dado.** A nota de configuração e a de capacidade sobem quando uma etapa da coleta falha? A versão 0.8.1 corrigiu isso para a nota de medição; confira as outras duas.
- **A3. Caminho único para "informação insuficiente".** Do fato até a tela, existe um caminho consistente para esse estado, ou cada regra resolve do seu jeito? Proponha a menor mudança que dê a ele uma representação única sem quebrar a nota.
- **A4. Veredito de gargalo na medição de FPS.** Em `internal/perf/accum.go:86-93`, um quadro é "limitado pela GPU" quando `GPUBusy >= 0,90 × tempo de quadro` (`stats.go:22`). Verifique: o que acontece quando as colunas faltam no CSV (os nomes aceitos estão em `csv.go:53-61`); que veredito aparece quando poucos quadros puderam ser classificados; se o estado do agendamento de GPU acelerado por hardware (HAGS) é lido e considerado no veredito (há relatos de que ele altera o tempo de GPU medido; não afirme isso sem citar a fonte); se jogos em OpenGL ou Vulkan produzem colunas que mudam o veredito.
- **A5. Estatística do antes e depois.** Teste t de Welch com ajuste de Holm (`compare.go`, `trial.go`), com 3 medições de cada lado. Verifique graus de liberdade, variância zero, amostras idênticas, e se "sem diferença detectável" pode ser mostrado como "o ajuste não faz nada".
- **A6. Temperatura.** PC sem zonas térmicas expostas: o diagnóstico diz que não leu, ou diz que a temperatura está normal? Confira `internal/diag/thermal.go` e as condições da medição em `internal/perf/conditions.go`.
- **A7. Falha do PresentMon.** Em `internal/app/perf.go:564-575`: a mensagem ao usuário distingue "o jogo não estava em primeiro plano" de "o PresentMon não iniciou" (por exemplo, sem o Visual C++ Redistributable, código de saída 0xC0000135)?
- **A8. O teste que falha com o detector de corrida.** `TestCaptureRecordsConditionsAndSensors` (`internal/app/perfsensors_test.go:145`) entrega sensores vazios quando a execução fica mais lenta. É só tempo do teste, ou há uma janela real em que a medição termina antes de a amostra de sensores ser juntada ao resultado?

## Formato do relatório

Máximo de 300 linhas. Não cole funções inteiras; cite `arquivo:linha` e, se preciso, até 10 linhas de código.

Comece com um parágrafo de veredito geral. Depois uma tabela com uma linha por achado:

| ID | Severidade | Arquivo:linha | O que acontece | Confiança | Critério de aceitação |
|---|---|---|---|---|---|

- **ID:** A-01, A-02...
- **Severidade:** crítica, alta, média ou baixa.
- **Confiança:** alta (li o caminho inteiro), média (li parte) ou baixa (inferência).
- **Critério de aceitação:** como um teste automático provaria que foi corrigido.

Depois da tabela, uma seção curta "Descartado" com as hipóteses que você verificou e não se confirmaram, com `arquivo:linha`. Feche com "Validar no Windows": o que não dá para afirmar só lendo.
