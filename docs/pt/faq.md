---
title: "FAQ do fbrescue: dados de um banco Firebird corrompido"
description: "Respostas curtas sobre recuperar um banco Firebird danificado com o fbrescue: sem servidor, sem alterar o arquivo, versões, preço, limites."
lang: pt-BR
faq:
  - q: Dá para extrair os dados de um banco Firebird sem o servidor Firebird?
    a: >-
      Sim. O fbrescue lê o arquivo `.fdb` diretamente, sem o motor e sem o `fbclient`, e exporta cada
      tabela para CSV, SQL (comandos INSERT) ou JSON Lines. É um único executável para Windows, Linux e
      macOS. Funciona também quando o banco já não abre e o `gbak` não consegue fazer o backup. Passo a
      passo: [abrir um arquivo .fdb sem o Firebird](../open-fdb-without-firebird/).
  - q: O fbrescue altera ou conserta o meu arquivo de banco?
    a: >-
      Não. Ele abre o arquivo só para leitura e nunca grava nele; um teste no CI roda todos os comandos em
      cópias somente leitura de bancos reais e confere que nada mudou. O fbrescue extrai os dados para
      arquivos que você carrega num banco novo; ele não conserta o original. Mesmo assim, trabalhe numa
      cópia: outras ferramentas que você testar podem gravar nela.
  - q: Quais versões do Firebird ele suporta?
    a: >-
      Firebird 2.5, 3.0, 4.0 e 5.0 (ODS 11.2 a 13.1), incluindo os tipos do Firebird 4 (INT128, DECFLOAT,
      TIME e TIMESTAMP WITH TIME ZONE), BLOBs e bancos multiarquivo. InterBase e Firebird 1.x, 2.0 e 2.1
      não são suportados.
  - q: É gratuito?
    a: >-
      Sim. O fbrescue é de código aberto, sob a licença MIT: sem licença por banco, sem limite de linhas
      ou de bancos.
  - q: Devo tentar o gfix e o gbak primeiro?
    a: >-
      Sim. Numa cópia do arquivo rode `gfix -v -full -n`, depois `gfix -mend -full -ignore` e
      `gbak -b -ig -g`. Se o backup terminar e restaurar, você recupera o banco inteiro, com índices,
      triggers e procedures. Use o fbrescue quando o gbak ainda parar.
  - q: Existe uma alternativa gratuita ao IBSurgeon FirstAID?
    a: >-
      Para extrair os dados, sim: o fbrescue. O FirstAID é uma ferramenta comercial para Windows que pode
      consertar o arquivo do banco no lugar ou extrair os dados, e suporta InterBase e versões antigas do
      Firebird. O fbrescue é gratuito, só lê o arquivo, roda em Windows, Linux e macOS e só extrai dados,
      do Firebird 2.5 ao 5.0. Veja a [comparação](../compare/).
  - q: O que o fbrescue não recupera?
    a: >-
      Índices, triggers, stored procedures, views, usuários, valores de generators e constraints: ele
      exporta as definições das tabelas (schema.sql) e as linhas. Colunas do tipo array ainda não são
      exportadas. Linhas em páginas zeradas ou cortadas do arquivo se perdem; cada linha que ele não
      conseguiu ler é listada em `_errors.jsonl`.
  - q: Dá para recuperar linhas apagadas ou sobrescritas?
    a: >-
      Muitas vezes sim, se as versões antigas dos registros ainda estão no disco (antes da coleta de lixo):
      `--include-deleted` grava as linhas apagadas num arquivo separado, e `--versions all` grava todas as
      versões de cada linha com a transação e o estado dessa transação.
  - q: E se as tabelas de sistema estiverem danificadas?
    a: >-
      Tabelas cujas definições se perderam em parte são exportadas como `TABLE_<id>` com colunas
      `FIELD_<n>`. Se os próprios formatos dos registros se perderam, passe os comandos CREATE TABLE do banco
      com `--ddl schema.sql` (o script de criação, a saída do `isql -x` ou uma exportação anterior do
      fbrescue), e o fbrescue calcula o layout dos registros a partir deles.
  - q: Quanto dos dados ele recupera?
    a: >-
      Depende do dano. Em cópias de um banco de teste danificadas de 13 maneiras diferentes no Firebird 2.5
      a 5.0, ele recupera 100% das linhas quando páginas de ponteiros, de índice, do inventário de
      transações ou o cabeçalho são zerados, e tudo menos as linhas da página danificada quando páginas de
      dados se perdem. A tabela completa está no [README](https://github.com/formeo/fbrescue#recovery-rate).
  - q: Como conseguir ajuda sem enviar os meus dados?
    a: >-
      Rode `fbrescue report copia.fdb` e anexe a saída a uma
      [issue no GitHub](https://github.com/formeo/fbrescue/issues), pode ser em português. O relatório
      descreve o dano sem nenhum dado, com os nomes de tabelas e colunas trocados por números.
  - q: O texto sai com acentos quebrados. O que fazer?
    a: >-
      O banco provavelmente guarda texto em `CHARACTER SET NONE`, e os bytes estão na codificação do
      programa que os gravou. Informe-a na exportação: `--charset WIN1252` para a maioria dos programas
      Windows brasileiros, ou `--charset ISO8859_1`. O fbrescue então grava tudo em UTF-8.
---

# FAQ do fbrescue

{% for item in page.faq %}
## {{ item.q }}

{{ item.a }}
{% endfor %}

Mais: [passos de recuperação]({{ '/pt/' | relative_url }}), [mensagens de erro]({{ '/pt/errors/' | relative_url }}), [baixar](https://github.com/formeo/fbrescue/releases/latest).
