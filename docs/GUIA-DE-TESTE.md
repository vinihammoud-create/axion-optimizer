# Roteiro de teste da 0.10.0 no notebook de jogo

Roteiro para validar a 0.10.0 num PC de verdade antes de publicar: o notebook G15 (Intel Core i5 de 13ª geração de notebook, 32 GB DDR5, NVIDIA RTX 3050 6 GB, tela acima de 120 Hz, CS2 instalado).

São dois níveis:

- **Nível 1, só leitura (cerca de 25 min):** nada no Windows muda. Pode ser feito na sua conta de sempre.
- **Nível 2, aplica e desfaz (cerca de 20 min, opcional):** feito numa conta de teste do Windows, para a sua configuração ficar de fora.

Anote tudo na [tabela de checagem](#o-que-anotar-e-mandar) do fim. Quando algo parecer estranho, tire uma captura de tela (Windows + Shift + S) antes de seguir.

## Preparação (5 min)

1. Ligue o notebook **na tomada**. Feche jogos e programas pesados.
2. Abra o link do rascunho da versão que você recebeu e baixe dois arquivos: `Axion-0.10.0-setup.exe` e `SHA256SUMS.txt`.
3. Confira se o instalador é o original. Abra o PowerShell na pasta do download e rode:
   ```
   Get-FileHash .\Axion-0.10.0-setup.exe
   ```
   O número que aparecer tem que ser igual ao da linha `Axion-0.10.0-setup.exe` do `SHA256SUMS.txt` (maiúsculas e minúsculas não importam). Se for diferente, pare e avise.
4. Crie um ponto de restauração do Windows: menu Iniciar › digite "Criar um ponto de restauração" › botão **Criar**. No Nível 1 ele é só precaução, porque nada muda; no Nível 2 ele é obrigatório.
   - Se o botão Criar estiver cinza, a Proteção do Sistema está desligada. Ligá-la muda uma configuração do Windows: decida você. Sem ela, não faça o Nível 2.

## Nível 1: só leitura (cerca de 25 min)

Pode ser na sua conta principal. Nada aqui muda o Windows: a análise só lê, a medição de FPS só lê os eventos do Windows (não mexe no jogo) e o relatório é um arquivo que você guarda.

### Instalar

1. Anote a escala da tela: Configurações › Sistema › Tela › Escala (por exemplo, 125% ou 150%).
2. Abra o `Axion-0.10.0-setup.exe`.
3. Se aparecer **"O Windows protegeu o computador"**, tire uma captura, clique em **Mais informações** e depois em **Executar assim mesmo**.
   - Se aparecer **"O controle inteligente de aplicativos bloqueou um aplicativo"**, pare aqui e anote. Para passar, seria preciso desligar esse controle do Windows, o que muda a sua configuração: decida você.
4. Aceite o pedido de administrador.
5. Em cada página do instalador, anote o título e se algum texto aparece cortado ou sem caber na tela. As páginas esperadas são: **O que o Axion faz**, a licença (MIT, precisa aceitar), a pasta de instalação, o atalho na área de trabalho (opcional, vem desmarcado), o resumo antes de instalar e a página final com **Abrir o Axion Optimizer** marcado.
   - Se aparecer a página **"WebView2 Runtime não encontrado"**, anote: é um resultado importante.
6. Na última página, deixe **Abrir o Axion Optimizer** marcado e clique em Concluir.

### Abrir

7. O esperado é o Axion abrir numa **janela própria**, não numa aba do navegador. Anote como abriu.
   - Se abrir no navegador, ou se aparecer no topo o aviso **"Aberto no navegador porque a janela própria do Axion não abriu…"**, tire uma captura e anote.
8. Escolha o perfil **Jogo competitivo** e espere a análise terminar (alguns segundos).

### Conferir a análise

9. **Início:** veja a nota e abra **Seu PC em detalhes**. Confira:
   - a placa **NVIDIA GeForce RTX 3050** aparece (a de vídeo dedicada), além da integrada da Intel;
   - o processador e os 32 GB de memória estão certos;
   - o PC é reconhecido como notebook.
   - Se aparecer "N partes não lidas" ao lado da nota, clique e anote quais partes.
10. **Diagnóstico:** leia cada alerta. Anote o título de todo alerta que parecer errado para este notebook. O que se espera:
    - **Não** deve aparecer o alerta "Microcode …: atualize a BIOS (instabilidade Intel 13ª/14ª geração)". Esse problema da Intel vale só para processadores de desktop; o seu é de notebook. Se aparecer, é um erro do Axion: capture a tela.
    - Nenhum alerta deve dizer que a tela está abaixo da frequência máxima, se ela já estiver na taxa máxima (veja o passo 11).
11. Ainda no Diagnóstico, abra **Dados coletados deste PC** e confira:
    - **Monitores:** a tela integrada com a taxa atual e a máxima. A esperada é acima de 120 Hz. Se a taxa atual estiver abaixo da máxima, só anote: não mude nada.
    - **Processador e jogo** e **Temperatura (o que o Windows informa):** anote o que aparece como "não lido". Num notebook, "não lido" ou "nenhuma exposta pelo firmware" é aceitável; o que não pode é um valor errado aparecer como certo.
    - **Segurança:** TPM, Secure Boot e anti-cheats aparecem com um valor ou como "não lido", nunca em branco.
12. **Perfil do driver NVIDIA:** vá em **Otimizar**, digite "driver NVIDIA" em **Buscar otimização…** e olhe os filtros **Não se aplicam** e **Todas** (por exemplo, o item "V-Sync do driver NVIDIA de volta ao padrão"). Cada item mostra, embaixo do título, o motivo:
    - "nada a corrigir no perfil do driver deste PC" ou o item disponível: o perfil foi **lido** (o esperado);
    - "o perfil do driver NVIDIA não foi lido: …": anote o motivo inteiro.
    - Não aplique nada aqui.
13. **Jogos › Counter-Strike 2:** no quadro **Instalação**, anote só para você o número que aparece no caminho da **Configuração de vídeo** (`…\userdata\NÚMERO\730\…`). É a pasta da sua conta da Steam; ele serve para comparar no Nível 2. Não precisa mandar esse número.

### Medir o FPS no CS2 (cerca de 5 min)

14. Abra a Steam com a sua conta normal e abra o CS2. Entre num mapa **offline** (prática com bots ou um mapa de benchmark da Oficina), sem partida online.
15. No Axion: **Medir › Desempenho (FPS)**. Escolha o `cs2.exe`, duração **60 s** e o rótulo "teste". Clique em **Iniciar medição**.
16. Volte para o jogo e aperte **Scroll Lock** (ou espere a contagem, se escolheu a contagem regressiva). Ande pelo mapa normalmente até a medição parar sozinha.
17. Feche o jogo, volte ao Axion e anote o FPS médio, o 1% low e o que a página diz que limitou (placa de vídeo, processador ou "não dá para apontar").

### Exportar o relatório anônimo

18. Clique em **Exportar relatório** (no Início). Escolha **Dados (JSON)** e **não** marque "Relatório completo". Assim o nome do PC, a conta do Windows, a conta da Steam e os endereços de rede saem trocados por marcadores.
19. Anote onde o arquivo `axion-relatorio-….json` foi salvo (em geral, a pasta Downloads) e guarde para mandar ao Claude. Se quiser ver o resumo você mesmo, exporte de novo como **Página (HTML)**.

### Desinstalar

20. Feche o Axion.
21. Configurações › Aplicativos › **Axion Optimizer** › Desinstalar. Anote cada pergunta que aparecer, com o texto. O esperado, sem nenhum ajuste aplicado:
    - o pedido de administrador e a confirmação para remover o programa;
    - **nenhum** aviso de "alterações ativas" (se aparecer um, anote o número que ele diz);
    - a pergunta **"Apagar também a pasta de dados do Axion?"**: responda **Não**. A pasta guarda as medições; se você fizer o Nível 2, ela continua sendo usada.
22. Confira que o atalho do menu Iniciar sumiu.

Fim do Nível 1. Mande o que tiver até aqui, mesmo que não faça o Nível 2.

## Nível 2: aplicar e desfazer, numa conta de teste (cerca de 20 min, opcional)

Aqui o Axion muda coisas de verdade e desfaz em seguida. Para a sua configuração ficar de fora, use uma conta separada do Windows e uma conta separada da Steam.

### O que uma conta separada protege, e o que não

Parte do que o Axion muda fica no perfil da conta do Windows; o resto vale para o PC inteiro, em qualquer conta. Por isso o roteiro termina com **Desfazer tudo** e há o ponto de restauração.

| Fica só na conta de teste (não afeta a sua) | Vale para o PC todo, mesmo feito na conta de teste |
|---|---|
| Aceleração do mouse ("Mouse 1:1") | Plano e modo de energia |
| Atalhos de acessibilidade (Shift 5 vezes etc.) | Hibernação e Inicialização Rápida |
| Modo de Jogo, Game Bar e gravação em segundo plano | Serviços e tarefas agendadas do Windows |
| Preferência de placa de vídeo por jogo | Rede: placas, TCP e DNS |
| Modo de tela cheia por jogo | Opções de boot |
| | Perfil global (e por jogo) do driver NVIDIA |
| | Frequência do monitor |
| | Prioridade do cs2.exe ("Perfil do cs2.exe no Windows") |

O preset do CS2 grava no `cs2_video.txt`, que fica separado **por conta da Steam**. Com uma segunda conta da Steam, o seu arquivo personalizado não é tocado.

### Preparar a conta de teste

1. Na sua conta: feche o Axion e a Steam.
2. Crie uma conta local do Windows **administradora**: Configurações › Contas › Outros usuários › Adicionar conta › "Não tenho as informações de entrada dessa pessoa" › "Adicionar um usuário sem uma conta Microsoft". Depois, nessa conta, **Alterar tipo de conta › Administrador**. (No Windows 10 o caminho começa em Configurações › Contas › Família e outros usuários › Adicionar outra pessoa a este PC.)
   - Tem que ser administradora: o Axion só funciona com permissão de administrador. Uma conta padrão ou de visitante não serve.
3. **Saia** da sua conta (menu Iniciar › seu nome › Sair; não use "Trocar de usuário", para a sua Steam não ficar aberta) e entre na conta de teste.
4. Abra a Steam e entre com uma **segunda conta da Steam**, só para o teste (pode ser criada na hora; o CS2 é gratuito). Abra o CS2 uma vez, para ele criar o arquivo de vídeo dessa conta, e feche.
5. Instale o Axion de novo com o `Axion-0.10.0-setup.exe` (o mesmo arquivo; não precisa baixar de novo) e abra.
6. **Jogos › Counter-Strike 2 › Instalação:** confira que o número em `…\userdata\NÚMERO\…` é **diferente** do que você anotou no passo 13 do Nível 1. Se for igual, não aplique o preset: anote e pule os passos 12 e 13 abaixo.
7. **Otimizar › Histórico › Criar ponto de restauração.** Anote se o Axion disse que criou ou se avisou de algo.

### Aplicar um ajuste e desfazer

8. Veja como o mouse está antes: Painel de Controle › Mouse › aba **Opções de ponteiro** › **"Aprimorar precisão do ponteiro"**. Numa conta nova, ela costuma vir marcada. Anote.
9. No Axion: **Otimizar** › categoria **Mouse e teclado** › ligue o interruptor de **"Mouse 1:1 (sem 'Aprimorar precisão do ponteiro')"**.
10. Abra de novo a aba Opções de ponteiro: a caixa deve estar **desmarcada**.
11. **Otimizar › Histórico:** o item aparece com o valor anterior. Clique em **Desfazer** e confira que a caixa voltou a ficar como estava no passo 8.

### Opcional: preset do CS2 e "Aplicar as recomendadas"

12. Com o CS2 fechado: **Jogos › Counter-Strike 2 › Preset competitivo**. Clique em **Aplicar preset** e confira que o caminho na confirmação é o da conta da Steam de teste (o número do passo 6). Aplique.
13. Abra o CS2 com a Steam de teste, veja em Vídeo › Avançado se as opções mudaram, feche o jogo e clique em **Desfazer preset**.
14. **Início › Aplicar as recomendadas:** leia a lista, anote quais itens ela traz e aplique. Se o Axion pedir para reiniciar, reinicie e entre de novo na **conta de teste**.
    - Se a tela estiver abaixo da taxa máxima, vai aparecer **"Manter esta frequência?"**: responda conforme o que você vê e anote o que aconteceu. Não baixe a taxa da tela só para provocar essa pergunta.

### O aviso do desinstalador, com um ajuste ainda aplicado

15. Este passo precisa de pelo menos um ajuste aplicado. Se você pulou os passos 12 a 14, ligue de novo o "Mouse 1:1" (passo 9). Depois **feche o Axion** e vá em Configurações › Aplicativos › **Axion Optimizer** › Desinstalar.
    - O esperado é um aviso dizendo **quantas alterações continuam ativas** e recomendando desfazer antes. Anote o texto e o número.
    - Clique em **Não**. A desinstalação tem que parar ali: confira que o atalho do menu Iniciar continua lá e que o Axion abre.
    - Se o programa for removido mesmo com o "Não", anote: é um erro do instalador. Instale de novo com o mesmo `Axion-0.10.0-setup.exe` (o histórico fica guardado) e siga.

### Desfazer tudo e apagar a conta

16. Ainda **na conta de teste**: **Otimizar › Histórico › Desfazer tudo**. Isso tem que ser feito nela: o que foi mudado no perfil da conta de teste só é desfeito por ela. Se o Axion pedir para reiniciar, reinicie e entre de novo na conta de teste.
17. Confira que o botão aparece como **Desfazer tudo (0)**, sem nada ativo. Anote qualquer item com "falhou" ou "falta desfazer".
18. Desinstale o Axion (Configurações › Aplicativos). Agora o aviso de "alterações ativas" não deve aparecer. Na pergunta da pasta de dados, responda **Não** se quiser manter as medições, ou **Sim** para apagar tudo.
19. Saia da conta de teste e entre na sua. Se a Steam pedir para escolher a conta, escolha a sua.
20. Apague a conta de teste: Configurações › Contas › Outros usuários › conta de teste › **Remover** › **Excluir conta e dados**.
21. Se algo do seu Windows parecer diferente, use o ponto de restauração da Preparação: menu Iniciar › "Criar um ponto de restauração" › **Restauração do Sistema**.

## O que anotar e mandar

Mande três coisas:

1. **A tabela abaixo preenchida** (pode ser copiada para um bloco de notas).
2. **As capturas de tela** das telas estranhas, com o número do passo no nome do arquivo.
3. **O relatório anônimo** (`axion-relatorio-….json`) do passo 19 do Nível 1.

| Passo | Item | Esperado | O que aconteceu |
|---|---|---|---|
| Prep. 3 | SHA-256 do instalador | Igual ao do `SHA256SUMS.txt` | |
| 1 | Escala da tela | (anotar) | |
| 3 | Aviso do Windows ao abrir o instalador | "O Windows protegeu o computador", com "Executar assim mesmo" | |
| 5 | Páginas do instalador | Textos inteiros, nada cortado; sem a página do WebView2 | |
| 7 | Como o Axion abriu | Janela própria, sem o aviso de navegador | |
| 9 | Placa de vídeo | RTX 3050 aparece, além da integrada | |
| 9 | Notebook | Reconhecido como notebook | |
| 9 | Partes não lidas | Nenhuma (se houver, quais) | |
| 10 | Alerta Intel 13ª/14ª geração | Não aparece | |
| 10 | Alertas que parecem errados | (títulos) | |
| 11 | Taxa da tela | Atual e máxima acima de 120 Hz | |
| 11 | Itens "não lido" em Dados coletados | (quais) | |
| 12 | Perfil do driver NVIDIA | Lido ("nada a corrigir" ou item disponível) | |
| 17 | FPS no CS2 (60 s) | Medição completa, com média, 1% low e o que limitou | |
| 19 | Relatório anônimo | Arquivo JSON salvo | |
| 21 | Perguntas da desinstalação | Sem aviso de alterações ativas; pergunta da pasta de dados | |
| N2-6 | Pasta da Steam de teste | Número diferente do da sua conta | |
| N2-7 | Ponto de restauração pelo Axion | Criado | |
| N2-10 | Mouse 1:1 aplicado | Caixa desmarcada | |
| N2-11 | Mouse 1:1 desfeito | Caixa como estava antes | |
| N2-13 | Preset do CS2 (opcional) | Muda no jogo e volta com "Desfazer preset" | |
| N2-14 | Aplicar as recomendadas (opcional) | (itens da lista; pediu reinício?) | |
| N2-15 | Desinstalar com ajuste aplicado | Aviso com o número de alterações ativas; com "Não", nada é removido | |
| N2-17 | Desfazer tudo | Nada ativo, nenhum "falhou" | |
| N2-18 | Desinstalação depois do teste | Sem aviso de alterações ativas | |

## Se algo der errado

- **Desfazer:** Otimizar › Histórico › Desfazer (um item, um lote ou tudo). O que foi feito na conta de teste é desfeito estando nela.
- **Restauração do Sistema:** volte ao ponto criado na Preparação.
- **Registros:** a pasta `C:\ProgramData\Axion` tem o `axion.log`, que explica o que aconteceu. Ele não é anônimo: confira antes de mandar.

O que depende de jogo, placa NVIDIA e anti-cheat reais (medição de FPS, perfil do driver NVIDIA, preset do CS2) até agora só foi testado no simulador. Este roteiro é o primeiro teste deles num PC de verdade.
