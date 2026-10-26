---
title: "Erro do Firebird: decompression overran buffer (179)"
description: "O que significa o erro do Firebird \"decompression overran buffer (179)\" — um registro danificado — e como salvar o resto da tabela quando gfix e gbak falham."
lang: pt-BR
---

# `internal Firebird consistency check (decompression overran buffer (179))`

A mensagem completa costuma ser `internal Firebird consistency check (decompression overran buffer (179), file: sqz.cpp line: ...)`, em uma consulta, no `gbak` ou no `gfix -v`.

## O que significa

O Firebird guarda cada registro comprimido com uma codificação simples de repetições (run-length). Quando ele descomprime um registro e o resultado não cabe no layout do registro, os bytes gravados estão danificados: por um disco com defeito, uma cópia malfeita, uma queda no meio de uma gravação ou uma página sobrescrita com outra coisa. O motor interrompe o comando que tocou no registro, então um `SELECT` ou um backup dessa tabela falha sempre no mesmo ponto. Os outros registros da tabela geralmente estão bons.

{% include steps-pt.md %}

O `gfix -mend` é a resposta do motor justamente para este erro: ele marca o registro danificado para que o próximo backup o pule.

{% include fbrescue-pt.md %}

Neste erro o fbrescue lê todos os registros da tabela, lista em `_errors.jsonl` os que não descomprimem, com página e linha, e exporta o resto. Um registro que continua em outra página (registros maiores que uma página) só se perde se um dos seus pedaços estiver danificado.

## Veja também

- [`wrong record length (183)`](../wrong-record-length/) — o mesmo tipo de dano, percebido mais adiante na leitura
- [O backup do `gbak` falha](../gbak-backup-fails/)
- [Todos os erros](../)
