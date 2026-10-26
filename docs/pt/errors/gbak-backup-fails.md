---
title: "O gbak não faz backup de um banco Firebird danificado"
description: "O gbak para com \"Exiting before completion due to errors\" em um banco Firebird danificado: opções que vão mais longe e o que fazer quando ainda falha."
lang: pt-BR
---

# `gbak: ERROR` … `gbak:Exiting before completion due to errors`

## O que significa

O `gbak` faz o backup através do motor do Firebird, tabela por tabela e linha por linha. O primeiro registro ou página que o motor não consegue ler interrompe o backup inteiro, e a mensagem antes de `Exiting before completion` diz o motivo: muitas vezes um [`internal Firebird consistency check`](../decompression-overran-buffer/), uma [página do tipo errado](../database-file-appears-corrupt/) ou um [erro de E/S](../io-error-during-read/). Tudo depois desse ponto falta no backup, mesmo que esteja intacto no arquivo.

## Opções que vão mais longe

- `-g` (sem coleta de lixo): o backup não tenta limpar versões antigas de registros, o que pode tocar em registros danificados que ele de outro modo não leria.
- `-ig` (ignorar checksums): pula os checksums de página que versões antigas gravam.
- `gfix -mend -full -ignore` antes do backup: marca os registros danificados para que o `gbak` os pule.
- `-v` mostra qual tabela estava sendo lida quando ele parou.

A ordem completa de passos:

{% include steps-pt.md %}

{% include fbrescue-pt.md %}

O fbrescue lê sem o motor, então um registro ou página danificada custa aquele registro ou página, não o resto do backup. Cada tabela é exportada para o seu próprio arquivo, e o `_errors.jsonl` diz o que ficou de fora e por quê.

## Veja também

- [Todos os erros](../)
- [Passos de recuperação]({{ '/pt/' | relative_url }})
