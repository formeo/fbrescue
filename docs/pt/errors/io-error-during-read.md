---
title: "Firebird: I/O error during \"read\" operation for file"
description: "Firebird \"I/O error during read operation\": um arquivo de banco truncado ou um disco com defeito. O que significa e como salvar os dados que ainda estão lá."
lang: pt-BR
---

# `I/O error during "read" operation for file`

No Windows a operação se chama `"ReadFile"`; a linha seguinte costuma ser `Error while trying to read from file`, às vezes com `Reached the end of the file` ou um erro do sistema operacional.

## O que significa

O motor pediu uma página ao sistema operacional e não a recebeu. Ou a página está além do fim do arquivo — o arquivo é menor do que o banco acha: uma cópia que parou no meio, um disco cheio, um download interrompido —, ou o disco não conseguiu lê-la.

Se o disco está falhando, **pare de usá-lo**: cada leitura pode piorar a situação. Primeiro copie o arquivo para outro disco, se preciso com uma ferramenta que pula setores ilegíveis (`ddrescue` no Linux), e trabalhe na cópia.

Se o arquivo está truncado, as páginas depois do fim se foram, e as linhas delas também. Tudo o que vem antes do corte pode ser salvo.

{% include steps-pt.md %}

O `gbak` costuma parar na primeira página que não consegue ler, mesmo quando a maior parte do banco está intacta.

{% include fbrescue-pt.md %}

Para um arquivo truncado, o `fbrescue scan` informa as páginas que são referenciadas mas já não estão no arquivo, e a quais tabelas elas pertencem. A exportação lê todas as páginas que ainda existem e lista as linhas e valores que não conseguiu ler; blobs cujas páginas sumiram são gravados como NULL, e o resto da linha é mantido.

## Veja também

- [`database file appears corrupt`](../database-file-appears-corrupt/)
- [Todos os erros](../)
