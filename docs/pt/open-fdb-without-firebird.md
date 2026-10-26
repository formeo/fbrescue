---
title: Abrir um arquivo .fdb do Firebird sem o Firebird instalado
description: "Veja as tabelas de um arquivo Firebird .fdb ou .gdb e exporte os dados sem um servidor Firebird instalado, com o fbrescue, uma ferramenta gratuita."
lang: pt-BR
---

# Abrir um arquivo `.fdb` sem o Firebird instalado

Você tem o arquivo de banco de um programa antigo — `.fdb`, às vezes `.gdb` — e nenhum servidor Firebird, nenhuma senha, talvez nem o próprio programa. Precisa ver o que tem dentro ou levar os dados para algo que você usa hoje.

O [fbrescue](https://github.com/formeo/fbrescue), uma ferramenta gratuita e de código aberto (licença MIT), lê o arquivo diretamente. Não precisa instalar nada: baixe o executável para Windows, Linux ou macOS em [Releases](https://github.com/formeo/fbrescue/releases/latest). Sem servidor, sem usuário, sem senha, e ele nunca grava no arquivo.

## Veja o que tem no arquivo

```
fbrescue info antigo.fdb      # versão do Firebird, tamanho de página, dialeto
fbrescue tables antigo.fdb    # comandos CREATE TABLE: tabelas, colunas, tipos
fbrescue scan antigo.fdb      # páginas, tabelas e quantidade de linhas; danos, se houver
```

## Extraia os dados

```
fbrescue export antigo.fdb --out dados                        # cada tabela em dados/<TABELA>.csv
fbrescue export antigo.fdb --out dados --table CLIENTES       # só algumas tabelas
fbrescue export antigo.fdb --out dados --format sql           # comandos INSERT para outro Firebird
fbrescue export antigo.fdb --out dados --format jsonl         # JSON Lines, para scripts
```

Todo formato vem com o `schema.sql`, as definições das tabelas. O texto é gravado em UTF-8. Bancos de programas brasileiros muitas vezes guardam texto em `CHARACTER SET NONE`; se a exportação avisar que os valores não são UTF-8 válido, informe a codificação real: `--charset WIN1252` (ou `ISO8859_1`, `DOS850`, `DOS437` …).

Próximos passos com os arquivos CSV: [abri-los no Excel ou carregá-los em outro lugar]({{ '/pt/firebird-to-csv/' | relative_url }}), ou [levar os dados para o PostgreSQL]({{ '/pt/firebird-to-postgresql/' | relative_url }}).

## Quais arquivos ele lê

Bancos do **Firebird 2.5, 3.0, 4.0 e 5.0** — qualquer que seja a extensão. Arquivos do InterBase e do Firebird 1.x (a maioria dos `.gdb` do começo dos anos 2000) usam um formato em disco mais antigo e não são suportados; o `fbrescue info` diz isso em vez de tentar adivinhar.

Se o arquivo está danificado — o programa parou de abri-lo, ou ele foi copiado de um disco com defeito —, veja [recuperar um banco corrompido]({{ '/pt/' | relative_url }}).

Use o fbrescue em bancos que são seus ou que você tem permissão para acessar.
