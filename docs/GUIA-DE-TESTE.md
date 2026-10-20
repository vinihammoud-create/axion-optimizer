# Guia de teste em notebook

Roteiro para uma semana de uso. Anote o que estranhar, junto com a tela, o que você fez e o que esperava.

## Antes de começar

1. Ligue o notebook **na tomada** e feche jogos e programas pesados.
2. Confira se a **Restauração do Sistema** está ligada: Painel de Controle › Recuperação › Configurar Restauração do Sistema.
3. Extraia o `.zip` numa pasta (ex.: `Documentos\Axion`) e abra o `Axion.exe`.
   - O Windows pode avisar "O Windows protegeu o computador", porque o programa ainda não tem assinatura digital. Confira o SHA-256 do `Axion.exe` com o `SHA256SUMS.txt` e clique em "Mais informações › Executar assim mesmo".

## Dia 1: só leitura

Nada aqui muda o PC.
1. Responda o perfil de uso e espere a análise.
2. **Início:** a nota, o hardware e o "Próximo passo" fazem sentido?
3. **Diagnóstico:** leia cada alerta. Algum está errado para este notebook? Anote o título. Abra "Dados coletados deste PC" e confira a seção **Temperatura**.
4. **Ctrl+K:** busque "rede", "fps" e "cs2".
5. **Exporte o relatório** (Início › Exportar relatório) e guarde o arquivo.

## Dia 2: aplicar e desfazer

1. **Otimizar › Histórico › Criar ponto de restauração.**
2. **Início › Aplicar as recomendadas:** revise a lista e aplique.
3. Reinicie se o Axion pedir. Use o notebook normalmente por algumas horas.
4. **Otimizar › Histórico:** desfaça um item e confira se ele volta ao que era.
5. Tire o carregador e tente desfazer o "Modo de energia", se ele foi aplicado. O Axion deve pedir para ligar o carregador.

## Dias 3 a 5: jogo e medição

1. **Medir › Desempenho (FPS):** meça o CS2 (ou outro jogo) por 60 s, na tomada.
2. **Jogos › Relatório da partida:** ligue para o seu jogo, jogue uma partida e feche o jogo. O relatório deve aparecer.
3. Se o notebook esquentar muito, veja se a medição mostra "o Windows limitou a frequência do processador por temperatura".
4. **Medir › Rede e ping:** rode o teste no Wi-Fi e, se puder, no cabo.

## Dias 6 e 7: manutenção

1. **Manutenção › Limpeza:** veja o que ela oferece e limpe só o que fizer sentido.
2. **Diagnóstico › Saúde do Windows:** confira os itens. O reparo de arquivos é opcional e leva alguns minutos.
3. Rode a análise de novo e compare a nota com a do Dia 1.

## Se algo der errado

- **Desfazer:** Otimizar › Histórico › Desfazer (um item, um lote ou tudo).
- **Restauração do Sistema:** volte ao ponto criado no Dia 2.
- **Pasta de dados e log:** `C:\ProgramData\Axion` (o `axion.log` explica o que aconteceu).

## O que mandar de volta

- O relatório exportado no Dia 1 e outro no Dia 7.
- A lista de anotações: tela, o que você fez, o que esperava e o que aconteceu.
