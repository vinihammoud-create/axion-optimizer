# Revisão de mudanças no catálogo

Vale para qualquer otimização nova ou alterada, venha de onde vier (outra pessoa,
um assistente de IA, um guia da internet). Os testes em
`internal/catalog/guard_test.go` pegam o que já está catalogado; **não conseguem
dizer se uma afirmação nova é verdadeira**. Isso é trabalho de leitura.

## Antes de aceitar

1. **Abra cada fonte e leia.** O texto da otimização diz o que a fonte diz? Uma
   página da Microsoft que descreve um mecanismo não prova que forçá-lo melhora a
   latência. Link que não abre, ou que abre outra página, é motivo para recusar.
2. **Confira a lista "O que não fazemos"** (`internal/catalog/excluded.go`). Se o
   valor ou a ideia já está lá, a mudança contradiz o projeto. Se for uma ideia
   nova que se recusa, registre-a lá com `Targets`, para o teste protegê-la.
3. **Processos do sistema** (`systemExes` em `profiles.go`) nunca recebem prioridade,
   afinidade ou perfil.
4. **Evidência e risco devem ser honestos.** Evidência *alta* pede medição pública ou
   documentação do ganho, não só do mecanismo. Mexer na chave de hardware de um
   dispositivo (`Enum\...`, `Interrupt Management`) nunca é risco *seguro* nem 1 clique.
5. **Desfazer.** Toda operação precisa guardar o valor anterior e voltar ao estado exato
   (o teste `TestApplyVerifyRevertEveryTweak` cobre).
6. **Texto em português claro**, sem prometer o que não foi medido ("elimina",
   "garante", "microssegundos") e sem afirmar o que o mecanismo não faz.

## Depois

- `go vet ./cmd/... ./internal/...` e `go test ./internal/...` sem falhas novas.
- Se a mudança mexe no que o Windows faz, diga no LEIA-ME em "NOVO NA x.y".
