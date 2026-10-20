# Segurança

O Axion roda como administrador e mexe em configurações do Windows. Por isso, uma falha de segurança nele é levada a sério.

## Como avisar

Não abra uma issue pública para falhas de segurança. Use o botão **"Report a vulnerability"**, na aba **Security** deste repositório; ele abre um aviso privado para o mantenedor.

Descreva no aviso:
- o que acontece;
- como reproduzir;
- a versão do Axion e do Windows.

## O que o Axion promete

- **Nada de driver de kernel.** O Axion não abre o processo do jogo para escrita, não faz injeção nem overlay.
- **Nada de PowerShell nem de arquivos de lote.** Só registro, WMI, a API do Windows e programas do próprio Windows, sempre chamados a partir da pasta do sistema.
- **Desfazer exato.** Toda mudança grava o valor anterior em um histórico assinado (HMAC) antes de ser aplicada, e o desfazer volta exatamente a ele.
- **Pasta de dados protegida.** `C:\ProgramData\Axion` só aceita gravação de administradores e do sistema; links e junções plantados ali são removidos antes do uso.
- **Interface só local.** Ela fica em `127.0.0.1` e exige um token; o servidor confere o cabeçalho Host e a origem de cada pedido.

## Fora do escopo

- Programas que o usuário instala pela página "Instalar programas": eles vêm do winget, de cada fabricante.
- Anti-cheats: o Axion não promete que nenhum anti-cheat vá aceitá-lo, porque as regras são deles e mudam.
