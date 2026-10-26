---
title: "Erro do Firebird: wrong record length (183)"
description: "O que significa o erro do Firebird \"wrong record length (183)\" e como salvar os dados da tabela quando gfix e gbak não passam do registro danificado."
lang: pt-BR
---

# `internal Firebird consistency check (wrong record length (183))`

## O que significa

Cada registro leva o número do seu formato (o layout da tabela no momento em que foi gravado) e se descomprime no tamanho desse formato. `wrong record length` significa que um registro se descomprimiu em outro tamanho: seus bytes, seu cabeçalho ou um dos seus fragmentos estão danificados. Assim como [`decompression overran buffer`](../decompression-overran-buffer/), é um dano em registros individuais, e o comando que lê o registro falha toda vez.

{% include steps-pt.md %}

{% include fbrescue-pt.md %}

Neste erro o fbrescue lê cada registro com o formato que ele indica. Um registro curto demais para o seu formato é listado em `_errors.jsonl` com página e linha; os outros registros da tabela são exportados. Tabelas alteradas muitas vezes não são problema: cada registro é lido com o layout com que foi gravado.

## Veja também

- [`decompression overran buffer (179)`](../decompression-overran-buffer/)
- [`database file appears corrupt`](../database-file-appears-corrupt/)
- [Todos os erros](../)
