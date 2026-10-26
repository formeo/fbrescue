---
title: Recuperar dados de um banco Firebird corrompido
description: "O que fazer quando um banco Firebird está corrompido e o gbak falha: primeiro as ferramentas do motor, depois o fbrescue, gratuito e sem servidor."
lang: pt-BR
software: true
---

# Recuperar dados de um banco Firebird corrompido

O seu banco Firebird parou de abrir, uma consulta falha com `internal Firebird consistency check` ou o `gbak` não consegue fazer o backup. Esta página traz a ordem de passos que salva o máximo de dados, do menos ao mais drástico. As [páginas de erros](errors/) explicam o que significam as mensagens mais comuns.

{% include steps-pt.md %}

{% include fbrescue-pt.md %}

## O que o fbrescue faz com cada dano

- **Registros danificados**: são pulados e listados; o resto da tabela é exportado.
- **Páginas danificadas ou zeradas**: as linhas dessas páginas se perdem, o resto do banco não. Páginas de dados que as páginas de ponteiros da tabela já não alcançam são encontradas lendo o arquivo inteiro.
- **Páginas fora do lugar** (a partir do Firebird 3 cada página guarda o próprio número): são lidas de onde estão.
- **Inventário de transações danificado** (`cannot find tip page`): as linhas ainda são exportadas; quando o estado de uma transação se perdeu, vale a versão mais nova, com um aviso.
- **Página de cabeçalho danificada**: o tamanho de página e a versão são deduzidos do resto do arquivo.
- **Definições de tabelas perdidas**: tabelas e colunas saem com nomes inventados (`TABLE_131`, `FIELD_2`). Se os próprios layouts dos registros se perderam, passe ao fbrescue os comandos CREATE TABLE do seu banco (`--ddl schema.sql`): o script que o criou, a saída do `isql -x` ou uma exportação anterior do fbrescue.

Quanto volta de cada tipo de dano foi medido em centenas de cópias danificadas de um banco de teste no Firebird 2.5, 3.0, 4.0 e 5.0: veja [a taxa de recuperação](https://github.com/formeo/fbrescue#recovery-rate).

O fbrescue extrai os dados; ele não conserta o arquivo no lugar. Índices, triggers, procedures e usuários não são exportados: crie-os de novo no banco novo. Como ele se compara ao gfix/gbak e ao IBSurgeon FirstAID: [comparação](compare/); respostas curtas: [FAQ](faq/).

## O banco não está danificado, só não há Firebird para abri-lo?

A mesma ferramenta exporta bancos íntegros: [abrir um arquivo .fdb sem o Firebird]({{ '/pt/open-fdb-without-firebird/' | relative_url }}), [exportar para CSV ou Excel]({{ '/pt/firebird-to-csv/' | relative_url }}), [levar os dados para o PostgreSQL]({{ '/pt/firebird-to-postgresql/' | relative_url }}).

## Erros comuns

{% include errors-pt.md %}
