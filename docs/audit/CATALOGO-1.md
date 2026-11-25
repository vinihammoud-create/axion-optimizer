# Auditoria do catálogo, parte 1: advanced, power, game, misc

Auditoria somente leitura (issue #27). Nenhum código foi alterado. Leitura feita em 25/11/2026 sobre o `main` no commit `246119d`.

Li linha por linha `internal/catalog/advanced.go`, `power.go`, `game.go` e `misc.go`. Para o desfazer li também `internal/tweaks/ops.go`, `opsx.go` e `profiles.go`; para as regras, `excluded.go`, `internal/tweaks/model.go` e o `REVISAO-DE-MUDANCAS.md`. Não rodei teste algum: nada foi executado no Windows nem no simulador.

## Como abri as fontes

- Cada URL foi aberta com `curl` (seguindo redirecionamentos) e o texto lido. Páginas que dependem de JavaScript, abri no navegador.
- Onde o servidor devolveu 403 ao meu cliente (TechPowerUp, VRChat Wiki, FACEIT, Eclypsium, Microsoft Security Blog), li a cópia do Internet Archive. Isso prova o conteúdo, não que o link ao vivo abre para o usuário.
- "Não abri" quer dizer que nem assim li a página (lista na seção final).

## Resumo

39 ajustes lidos (5 em `advanced.go`, 8 em `power.go`, 9 em `game.go`, 17 em `misc.go`). Nenhuma contradição com `excluded.go`. Nenhuma regra de hardware violada: `gpu.msi.mode` é moderado e fora do 1 clique, como pede o `REVISAO-DE-MUDANCAS.md`.

31 achados: 8 de confiança alta (F01, F02, F05, F06, F10, F18, F21, F30), 19 média e 4 baixa (F11, F15, F27, F29). A confiança de cada um é a do ponto mais forte dele.

Os mais graves: uma fonte que não abre e sustenta dois ajustes (F01); um ajuste cujo valor vem só de comentário de leitor de blog, contra a opinião do autor (F02); evidência "média" ou "alta" sem fonte que diga o que o texto afirma (F06, F18, F21, F28); e o desfazer da hibernação, que não volta ao estado exato (F10).

Convenção: `arquivo:linha` aponta o trecho; "Corrigir" é sugestão, não aplicada.

## advanced.go

**F01. `adv.vbs.off` e `adv.vmp.off`: a fonte `srcVMP` não abre.** `common.go:68`, usada em `advanced.go:20` e `advanced.go:60`. A URL do Microsoft Support redireciona para a página inicial do suporte (confirmado com `curl` e no navegador). O `adv.vmp.off` tem só essa fonte e evidência média (`advanced.go:60`); o texto "A Microsoft diz que… pode afetar o desempenho" (`advanced.go:15`, `advanced.go:58`) fica sem prova. Confiança: alta. Corrigir: achar a URL atual; sem ela, `EvLow` no `adv.vmp.off` e tirar "A Microsoft diz".

**F02. `adv.timer.global`: o valor não tem fonte, e o autor citado desaconselha.** `advanced.go:79-83`. `GlobalTimerResolutionRequests` não está na página da Microsoft (`srcTimer`). Em `srcTimerDawson` aparece só num comentário de leitor, e Bruce Dawson responde que não recomendaria (exceto para teste) e que software "definitivamente não deveria" gravar isso em máquina de usuário. Também sem fonte: "lido pelo kernel do Windows 11 e Server 2022+" (um leitor diz que só funciona no 11; Server 2022 não aparece), "medições públicas inconsistentes" e "0,5 ms da Sessão de Jogo". Confiança: alta. Corrigir: dizer que é relato de leitor, não documentado; citar a ressalva do Dawson; remover as três afirmações sem fonte. A citação de `excluded.go:163` pede a mesma ressalva.

**F03. `adv.vmp.off`: "não afeta o anti-cheat" sem fonte.** `advanced.go:70`. Nenhuma fonte do item trata de anti-cheat e VMP. Confiança: média. Corrigir: "não verificamos o efeito no anti-cheat" ou fonte.

**F04. `adv.vbs.off`: a situação do FACEIT pode estar velha.** `advanced.go:18`. A página (cópia) é de 23/10/2025: VBS e IOMMU vão a "ondas crescentes" a partir de novembro de 2025, e o Windows 11 passa a ser obrigatório em 14/10/2026. "De parte dos jogadores, em etapas" pode já não valer em nov/2026. O bloqueio (`RequiresVBS`, `facts.go:874`) falha para o lado seguro, o que está certo. Confiança: média. Corrigir: reconferir a política atual e datar a afirmação.

**Sem achado.** `adv.mmcss`: a página do MMCSS confirma o arredondamento para múltiplo de 10 e que abaixo de 10 vira 20; o texto bate e é coerente com `excluded.go:62-64`. `adv.spectre`: os valores 3/3 batem com a KB4073119 e a perda maior em Haswell ou mais antigo bate com o blog da Microsoft (cópia; `srcSpectrePerf` dá 403 ao vivo). Ver F29 (baixa).

## power.go

**F05. Fonte do X3D tem título que a página não cumpre.** `common.go:34`, usada em `power.go:49`, `power.go:140` e `game.go:38`. O título promete "plano Equilibrado"; a página do TechPowerUp (cópia) cita só Modo de Jogo, Game Bar e driver de chipset. O plano Equilibrado vem da VRChat Wiki (wiki de comunidade), que remete a um post da AMD que o catálogo não cita. "O driver da AMD estaciona o CCD sem V-Cache" (`power.go:35`) sustenta cinco ajustes e não tem fonte da AMD. Confiança: alta (título), média (força da fonte). Corrigir: citar a página da AMD e corrigir o título.

**F06. `power.plan.balanced`, `.high` e `.ultimate`: `srcPowerOpts` não diz o que os textos atribuem.** `power.go:47`, `power.go:67-69`, `power.go:89-91`. A página (`common.go:30`) trata de perfis do gerenciador de energia do processador e não menciona Alto desempenho nem Ultimate. Sem fonte: "sobe o clock sob demanda em milissegundos", "frequência mínima de 100% e sem economia em PCIe/USB", "pensado pela Microsoft para estações de trabalho", "remove micro-economias". Confiança: alta. Corrigir: fonte dos planos de energia, ou assumir que é observação do projeto.

**F07. `power.mode.best`: a recomendação promete mais do que o texto admite.** `power.go:112` diz que o efeito de cada modo "é definido pelo fabricante do PC"; `power.go:127-129` recomenda "Sim" em qualquer PC, com "desempenho máximo na tomada". A fonte (`srcPowerSlider`) só descreve o modo como "favorece desempenho". Em notebook e PC antigo isso eleva calor e ruído sem ganho medido. Confiança: média. Corrigir: `RecNeutral` fora do X3D de 2 CCDs e sem "desempenho máximo".

**F08. `power.coreparking`: "ganho medido é pequeno" sem medição.** `power.go:138`. `srcCoreParking` só documenta o mecanismo (100% desliga o core parking). "Já desestaciona em milissegundos" e "aparece mais em CPUs antigas" não têm fonte. Confiança: média. Corrigir: "não achamos medição pública".

**F09. `power.usb.suspend` e `power.pcie.aspm`: evidência baixa, mas entram no 1 clique completo.** `power.go:158-168` e `power.go:170-184`. `OptionalEligible` (`model.go:181-183`) não exclui `EvLow`: com `OneClick: true`, `Risk: Safe` e sem `Group`, os dois são oferecidos (desmarcados) na camada completa. A Microsoft "recomenda fortemente não desativar" a suspensão seletiva (`srcUSBSuspend`, já citado no `Detail`), e a página do ASPM traz o aviso "administradores não devem alterar" (setting oculta), que o texto não repete. Além disso:
- `power.go:172`: "às vezes resolve microtravadas" não tem fonte.
- `power.go:159` diz que mouse e teclado já ficam fora da suspensão, mas a tag `entrada` (`power.go:163`) promete um benefício que o texto nega. `srcHIDSuspend` fala de dispositivos HID USB em geral, sem citar mouse e teclado.

Confiança: média. Corrigir: `OneClick: false`, repetir o aviso, tirar a tag e a frase das microtravadas.

**F10. `power.hibernate`: o desfazer não volta ao estado exato e o texto promete.** `power.go:186-201`, `opsx.go:241-246`, `opsx.go:274-299`.
- O snapshot guarda só "ligada ou desligada" e `HiberbootEnabled`. A própria fonte (`srcHibernate`) documenta `/hibernate /size` e `/type reduced|full`; quem tinha tipo ou tamanho próprio volta ao padrão ao desfazer. Confiança: média.
- `power.go:187` promete "garantindo que 'Desligar' reinicie drivers de verdade"; `srcPowerStates` só diz que kernel e drivers são preservados. "~40% da RAM" (`power.go:198`) e "alguns segundos mais lento" (`power.go:188`) não têm fonte nem medição. Confiança: alta.
- O mesmo `srcPowerStates` diz que o sleep híbrido de desktop usa o arquivo de hibernação contra queda de energia; o `Detail` não avisa dessa perda. Confiança: média.
- `EvMedium` sem fonte que documente o ganho.

Corrigir: guardar e restaurar tipo e tamanho; tirar "garantindo"; avisar do sleep híbrido; rebaixar a evidência.

**F11. Notebook: critério diferente entre planos.** `power.go:71-75` e `power.go:93-97`. O Ultimate é inaplicável em notebook, mas o Alto desempenho só recebe `RecNo`. Confiança: baixa. Corrigir: um critério só.

**Sem achado.** `power.plan.balanced` e `power.mode.best` (a operação). O desfazer de `SchemeOp` (`ops.go:515-690`) apaga a cópia do Ultimate só se foi criada pelo Axion, volta ao plano anterior e, se ele sumiu, ao Equilibrado. `overlayOp` (`power.go:225-285`) se recusa a ler e gravar na bateria, o que evita desfazer no modo errado. `PowerOp` (`ops.go:358-470`) guarda o valor anterior e confere se o plano ainda existe. Nota de baixa confiança: `PowerOp` grava o valor efetivo lido, então uma configuração sem entrada explícita passa a ter uma, com o mesmo comportamento.

## game.go

**F12. `game.dvr.history`: o texto vai além da fonte e a evidência é alta.** `game.go:52-55`. Li a página do Xbox Support no navegador (seção recolhida): a gravação em segundo plano "vem desligada por padrão" e "pode afetar o desempenho porque usa recursos do PC". O `Summary` diz que ela "grava continuamente" e "usa o codificador de vídeo da GPU o tempo todo", e `EvHigh` pede medição ou ganho documentado. `RecYes` em todo PC (`game.go:57-62`) vale também para quem já tem tudo desligado. Confiança: média. Corrigir: `EvMedium` e texto fiel à fonte.

**F13. `game.dvr.capture`: "custo medido é pequeno" sem medição.** `game.go:70-72`. Nenhuma fonte mede; "OBS/ShadowPlay não são afetados" também não tem fonte. Confiança: média.

**F14. `game.mode`: a leitura ignora outra chave que o próprio projeto cita.** `game.go:47`, `collect.go:1014`. `excluded.go:208` lista `AllowAutoGameMode = 0` como forma de desligar o Modo de Jogo, mas nem o ajuste nem a coleta leem essa chave: um PC com ela em 0 aparece como "Modo de Jogo ligado" sem estar (falso verde). Sem fonte: "Windows 10 (1703+)" (`game.go:36`; a página só diz "ligado por padrão"). `EvHigh` apoiada em frase qualitativa ("ajuda a obter taxa de quadros mais estável, conforme o jogo e o sistema"). Confiança: média. Corrigir: ler `AllowAutoGameMode`; sem ele, não concluir "ligado".

**F15. `game.hags`: afirmações sem fonte aberta.** `game.go:103-106`, `common.go:101-109`. "Requisito do DLSS Frame Generation" só tem a página da Nixxes (estúdio de jogos, não NVIDIA; não abri). "GTX 10 em diante, RDNA, Arc" (`game.go:104`) não está em `srcHAGS`, que fala de "GPUs recentes com driver WDDM 2.7". Já o `Detail` bate com `srcHAGS` ("os usuários não devem notar mudanças significativas"). `nvidiaFGCapable` casa "RTX 40/50/60" por substring: as Quadro Turing "RTX 4000/5000/6000", sem Frame Generation, casam. Confiança: baixa. Corrigir: fonte da NVIDIA; casar o nome com mais rigor.

**F16. `game.windowed`: valor de registro apresentado como documentado.** `game.go:136-137`. "É a mesma chave do botão em Configurações" não está em `srcWindowedOpt`, que descreve a opção, não o valor `SwapEffectUpgradeEnable` (a página confirma flip model, menos latência e VRR/Auto HDR). Confiança: média. Corrigir: "valor observado pelo Axion, não documentado". O desfazer (`profiles.go:167-225`) reescreve a string inteira: perde fragmentos sem "=", repetições e a ausência de ";" final, então não é idêntico byte a byte, e a chave `UserGpuPreferences`, se criada pelo ajuste, fica vazia (confiança baixa).

**F17. `game.refresh`: sem tela de confirmação e risco "seguro".** `game.go:200-204`, `opsx.go:678-717`, `native.go:248-252`. O ajuste testa com `CDS_TEST` e grava com `CDS_UPDATEREGISTRY`, sem o "Manter alterações?" de 15 s de Configurações > Tela, que o texto cita (`game.go:201`). Monitor, cabo ou TV que anuncia o modo mas fica sem imagem não volta sozinho. `EvHigh` é aceitável: a página da Microsoft documenta o benefício (menos atraso de entrada), de forma qualitativa. Confiança: média. Corrigir: reversão automática com confirmação, ou avisar no `Detail` e `OneClick: false`.

**Sem achado.** `game.hpet.fix` e `game.boottick.fix`: a página do BCDEdit diz que `useplatformclock`, `disabledynamictick` e `useplatformtick` são "só para depuração"; `srcQPC` confirma o custo maior da plataforma; alinhados com `excluded.go:134-136`; `bcdOp` guarda presente e valor e apaga se não existia. `game.ducking`: o texto é honesto sobre o valor 3 não documentado e o relato do LTSC (conferi o Q&A); só "Por padrão o Windows reduz 80%" (`game.go:90`) descreve o painel clássico e pode não valer hoje (baixa).

## misc.go

**F18. `privacy.telemetry.tasks`: nenhuma fonte cita as seis tarefas.** `misc.go:127-133`. `srcSchedTasks` é a página do cmdlet `Disable-ScheduledTask`; `srcTelemetry` não menciona Appraiser, CEIP nem Siuf. O `RiskNote` sobre o Appraiser também não tem fonte. `EvMedium` sem apoio. Confiança: alta. Corrigir: fonte que descreva as tarefas, ou `EvLow`.

**F19. `privacy.telemetry`: um dos dois valores não tem fonte; edição.** `misc.go:101-106`. `DoNotShowFeedbackNotifications` não aparece em `srcTelemetry` nem em `srcTelemetryCSP`, e o `Summary` promete desligar os pedidos de feedback, com evidência alta. O `Detail` diz "Obrigatório no Home/Pro"; a tabela do Policy CSP lista Pro, Enterprise e Education (não Home). `srcTelemetryCSP` confirma que 0 fora de Enterprise e Education equivale a 1. Confiança: média.

**F20. `privacy.diagtrack`: consequência sem fonte.** `misc.go:147-151`. "O Programa Windows Insider deixa de funcionar" não está em `srcTelemetry` nem em `srcServices` (guia do Windows Server, só descrição). Confiança: média.

**F21. `services.sysmain.hdd`: evidência média sem fonte.** `misc.go:321-325`. A única fonte (`srcServices`) diz só "mantém e melhora o desempenho". "Disco em 100% por minutos após o boot" e "em SSD é benéfico" não têm apoio. Confiança: alta. Corrigir: `EvLow` ou fonte.

**F22. `services.optional`: a fonte não cobre dois dos quatro serviços.** `misc.go:304-318`. `srcServices` não traz `Fax` nem `RetailDemo`. "Em Manual o Windows ainda inicia o serviço se algum programa precisar" (`misc.go:306`) só vale para serviço com gatilho ou iniciado por quem depende dele. O `Summary` diz "para Manual", mas `RetailDemo` vai a Desativado (`misc.go:316`). "Alguns MB de RAM" sem medição. Confiança: média.

**F23. `privacy.websearch`: o `Summary` afirma o que o `Detail` admite não ser documentado.** `misc.go:174-184`. `srcSearchPolicy` documenta a política só para o Explorador de Arquivos. O `Summary` e o `Recommend` ("deixa de ir para o Bing") dão o efeito no Iniciar como certo; `EvMedium` deveria ser `EvLow`. Confiança: média.

**F24. `privacy.widgets`: edição e "libera memória".** `misc.go:186-194`. A chave bate com o Policy CSP, que lista Pro, Enterprise e Education. "Libera memória" (`misc.go:191`) e "processos WebView2 carregados" (`misc.go:188`) não têm fonte nem medição. Confiança: média.

**F25. Políticas sem checagem de edição.** `privacy.activity` (`misc.go:158`), `privacy.websearch`, `privacy.widgets`, `update.do.p2p` (`misc.go:225`), `update.drivers.exclude` (`misc.go:245`). Os Policy CSP listam Pro, Enterprise e Education e nenhum `Applicable` lê a edição (só `privacy.telemetry` usa `f.OS.Edition`, para escolher o nível). No Home o item pode aparecer como aplicado sem efeito. Confiança: média. Corrigir: `Applicable` por edição, `Effect`, ou aviso no texto.

**F26. `security.wpbt`: efeito dado como certo, valor só em fontes de terceiros.** `misc.go:197-217`. O `Summary` (`misc.go:198`) afirma o bloqueio sem ressalva; o `Detail` admite valor não documentado. As fontes são WinUtil e persistence-info ("dica de @Harvesterify"); a Eclypsium (cópia) descreve o WPBT, não o valor. `Applicable` não verifica se existe tabela WPBT no firmware. `RecYes` fora de notebook (`misc.go:211`, `misc.go:213`) com evidência baixa, e `Risk: Safe`: bloquear o firmware pode barrar antirroubo e gestão de empresa. Confiança: média. Corrigir: ler a tabela WPBT para `Applicable`, `RecNeutral`, texto "segundo a WinUtil".

**F27. `update.drivers.exclude`: o `Summary` promete mais que a política.** `misc.go:246`. A política exclui drivers "das atualizações de qualidade" (Policy CSP), não "impede o Windows Update de trocar drivers" em geral. Confiança: baixa.

**F28. `ui.menudelay`: evidência alta sem a fonte do valor.** `misc.go:288-292`. `srcMenuDelay` documenta a API `SPI_SETMENUSHOWDELAY` (em ms), não o valor de registro `MenuShowDelay` nem o padrão 400. Confiança: média. Corrigir: `EvMedium` ou fonte do valor.

**F29. Pequenas inexatidões de texto.** Confiança: baixa.
- `ui.fileext` (`misc.go:278`): "deixando evidente um 'cheat.pdf.exe' malicioso"; a página só ensina a mostrar extensões.
- `privacy.ads` (`misc.go:84`): "sem nenhuma contrapartida"; `ScoobeSystemSettingEnabled` e `TailoredExperiences…` não têm fonte (o `Detail` admite só os do ContentDeliveryManager).
- `adv.spectre` (`advanced.go:119-122`): 5ª geração e geração desconhecida caem em "6ª geração ou mais novas", que não descreve esses casos.

**F30. `gpu.msi.mode`: diz "GPU dedicada", aplica na integrada.** `misc.go:41-58`. `Applicable` usa `PrimaryGPU()`, que em PC só com iGPU devolve a integrada (`facts.go:816-841`); o ajuste mexe então na chave PCI da iGPU, contra o título e o `Summary`. Confiança: alta. Corrigir: exigir `!g.Integrated` ou mudar o título. O risco está certo: moderado, reinício, fora do 1 clique e sem `DevicePriority` (`excluded.go:261-263`).

**F31. `gpu.mpo.off`: fonte não abriu e vale só para NVIDIA.** `misc.go:18-39`. A fonte é a página 5157 da NVIDIA (não abri, nem pela cópia). `Applicable` aceita qualquer GPU dedicada (`misc.go:23-27`), inclusive AMD e Intel, que a fonte não cobre. "É contestado se ainda funciona na 24H2/25H2" (`misc.go:20`) é honesto, mas sem fonte. Confiança: média.

**Sem achado.** `privacy.activity`: a política e o texto batem com o CSP, e a página de privacidade confirma "guardado só no PC" (edição em F25). `update.do.p2p`: `srcDO` confirma o padrão LAN (1) e o modo 0 "só HTTP". `ui.endtask`: a página "Configurações avançadas" lista a opção; o desfazer é registro simples. `privacy.ads`: o `Detail` já admite o que não é documentado (ver F29).

## Desfazer, em resumo

O desfazer de registro (`RegOp`), serviço (`SvcOp`, inclusive início atrasado, `services.go:75-83`), tarefas agendadas (`tasksOp`), BCD (`bcdOp`) e plano e modo de energia volta ao estado exato, incluindo "não existia" e a remoção das chaves criadas. As exceções estão em F10 (hibernação), F16 (`DirectXUserGlobalSettings` reescrito) e F17 (a mudança de Hz não tem reversão se a tela some).

## Não verificado

- Não rodei `TestApplyVerifyRevertEveryTweak` nem teste algum: o desfazer e as regras de `Applicable` e `Recommend` foram lidos, não executados.
- Não abri: NVIDIA 5157 (`srcMPO`, 403 ao vivo e na cópia); Nixxes (`srcNvidiaFG`, 403 ao vivo, 404 na cópia); o post da AMD sobre X3D e plano Equilibrado (só citado pela VRChat Wiki).
- Abri só por cópia do Internet Archive (ao vivo deram 403 ao meu cliente): TechPowerUp, VRChat Wiki, FACEIT (três páginas), Eclypsium e Microsoft Security Blog (`srcSpectrePerf`).
- Xbox Support (`srcGameMode`, `srcGameDVRBG`): li no navegador; na página de captura, só a seção "Gravação em segundo plano".
- Não conferi: se `OverlayTestMode = 5` ainda funciona na 24H2/25H2; se `SwapEffectUpgradeEnable`, `UserDuckingPreference = 3`, `DisableWpbtExecution` e `MenuShowDelay` se comportam como descrito; a versão em que "Finalizar tarefa" chegou (`MinBuild: 22631`, `misc.go:270`); o padrão do Modo de Jogo no Windows 10 1703; se o "Equilibrado" (`sys.go:349-351`) é o padrão do slider, já que a Microsoft cita "Melhor desempenho" (3af9b8d9…) como padrão.
- Os textos de `excluded.go` que não tocam estes quatro arquivos não foram auditados.
