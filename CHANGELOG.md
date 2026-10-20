# Histórico de versões

O histórico detalhado de cada versão, com o porquê de cada mudança, está no [`build/LEIA-ME.txt`](build/LEIA-ME.txt), que também vai dentro do pacote.

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
