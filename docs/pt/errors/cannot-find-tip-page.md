---
title: "Erro do Firebird: cannot find tip page (165)"
description: "O que significa \"cannot find tip page (165)\" — o inventário de transações do Firebird está danificado — e como exportar os dados quando o banco não abre."
lang: pt-BR
---

# `internal Firebird consistency check (cannot find tip page (165))`

## O que significa

As páginas do inventário de transações (TIP) registram, com dois bits por transação, se cada transação foi confirmada, desfeita ou ainda está ativa. Cada versão de linha leva o número da transação que a gravou, e o motor consulta esse número para decidir qual versão uma consulta enxerga. Quando uma página TIP falta, está zerada ou foi sobrescrita, o motor não consegue decidir e se recusa a continuar: muitas vezes o banco nem abre, e o `gbak` não consegue começar.

Os dados em si geralmente estão intactos: só o controle das transações se perdeu.

{% include steps-pt.md %}

{% include fbrescue-pt.md %}

Neste erro o fbrescue ainda exporta todas as tabelas. Transações mais antigas que a oldest interesting transaction do cabeçalho estão confirmadas por definição. Para as mais novas cujo estado se perdeu, o fbrescue fica com a versão mais nova da linha, o que está certo para toda transação que foi confirmada (quase todas), e a exportação informa em quantas linhas precisou fazer isso: entre elas pode haver alterações que foram desfeitas. Quando as páginas do inventário só estão fora do lugar, o fbrescue as encontra varrendo o arquivo.

## Veja também

- [`database file appears corrupt`](../database-file-appears-corrupt/)
- [Todos os erros](../)
