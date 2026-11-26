# Histórico de versões

## 0.10.0

Versão de teste, ainda sem assinatura digital. Windows 10 ou 11 de 64 bits, com conta de administrador.

**Instalar ou atualizar**

- Baixe o instalador `Axion-0.10.0-setup.exe` ou, se preferir não instalar nada, o ZIP portátil `Axion-0.10.0-win64.zip`. Os dois têm os mesmos arquivos.
- Antes de abrir, confira o SHA-256 do arquivo com o `SHA256SUMS.txt` desta página (PowerShell: `Get-FileHash .\Axion-0.10.0-setup.exe`). Sem assinatura, o Windows pode mostrar "O Windows protegeu o computador": depois de conferir, "Mais informações" › "Executar assim mesmo".
- Vindo da 0.9.1 (ZIP): feche o Axion e instale a 0.10.0 (ou extraia o ZIP novo). O histórico em `C:\ProgramData\Axion` continua valendo, então o que já foi aplicado ainda pode ser desfeito. Depois, a pasta do ZIP antigo e a pasta `%LOCALAPPDATA%\Axion\WebView2` (perfil antigo da janela) podem ser apagadas.
- Desinstalar não desfaz os ajustes. Para voltar ao que era, use **Histórico › Desfazer tudo** antes de desinstalar.

**O que mudou**

- **Instalador:** menu Iniciar, Configurações › Aplicativos, aviso quando falta o WebView2 Runtime. Na desinstalação, ele diz quantos ajustes do Axion ainda estão ativos e pergunta antes de apagar a pasta de dados (o padrão é manter).
- **"Não lido" no lugar de estado falso:** quando o Windows não responde a uma leitura, a tela diz "não lido" em tom neutro, a nota não aparece como "Excelente" e o Axion não aplica o ajuste que dependia dela (DNS, hibernação, recursos opcionais).
- **Desfazer por conta do Windows:** o que mudou no perfil de uma conta (mouse, acessibilidade, opções por jogo) só é desfeito por ela. Cancelar não interrompe um ajuste pela metade.
- **Frequência do monitor:** a nova taxa só fica gravada depois de "Manter"; sem resposta em 15 segundos, a tela volta sozinha à anterior.
- **Fontes revistas:** onde nenhuma fonte aberta confirma o texto, a evidência passou a baixa e a frase saiu. O SysMain em disco rígido e a volta do PowerThrottlingOff ao padrão saíram do 1 clique. A energia USB de mouse e teclado agora tem risco moderado, e o V-Sync forçado no driver NVIDIA deixou de vir marcado.
- **Interface:** o Modo simples ganhou "Desfazer mudanças" e "Sair do modo simples", os perfis Trabalho, Criação e Programação não destacam mais o CS2, e aparece um aviso quando a medição é feita na bateria.
- **Proteção:** o perfil da janela e a pasta de dados ficam onde só administradores alteram.

**Testado só no simulador:** a confirmação de 15 s da frequência do monitor (não num monitor real), a medição de FPS, o relatório da partida, o preset do CS2 e o perfil do driver NVIDIA (não há jogo, placa de vídeo dedicada nem anti-cheat reais no ambiente de teste). O instalador foi testado num Windows limpo da nuvem, instalando e desinstalando sem telas; ainda não num PC de usuário.

O detalhe de cada mudança, com o porquê, está no [`build/LEIA-ME.txt`](build/LEIA-ME.txt), que também vai dentro do pacote.

## 0.9.1

- **Interface nova:**
  - menu com sete entradas: Início, Otimizar, Diagnóstico, Jogos, Medir, Manutenção e Sobre;
  - busca Ctrl+K para qualquer página, ação ou otimização;
  - Início com a nota, o botão "Aplicar as recomendadas" e o ciclo analisar → aplicar → medir.
- **Aviso térmico do processador**, só com o que o Windows documenta e expõe sem driver:
  - desligamento por temperatura crítica;
  - throttling térmico confirmado pelo Windows;
  - limite do firmware;
  - zonas térmicas e limite de frequência por temperatura, também amostrados durante as medições de FPS e as partidas.
- **Ícone do programa** com a marca oficial. Os recursos do executável (ícone, manifesto e versão) passam a ser gerados por `cmd/axion-rsrc`.
- **Notebook:**
  - menos falsos alarmes: tela a 60 Hz na bateria, memória soldada, desligamento depois de tela azul;
  - "Modo de energia" com desfazer que espera o carregador.
- **Segurança:**
  - variáveis de ambiente que redirecionam código são limpas;
  - desinstaladores de pastas sem proteção rodam sem administrador;
  - a quarentena recusa junções;
  - o winget só é usado a partir da pasta protegida do pacote.
- **Licença MIT.**

## 0.9.0

- **Saúde do Windows:** telas azuis com orientação por código, drivers vulneráveis, reparo com DISM e SFC.
- **Prova no seu PC:** mede um ajuste antes e depois e só o mantém se houver melhora comprovada.
- **Perfil do driver NVIDIA:** leitura e correção com desfazer exato.
- **Relatório da partida** e **preset competitivo do CS2**.
- **Desfazer exato:** inicialização e DNS também passam a ter.
