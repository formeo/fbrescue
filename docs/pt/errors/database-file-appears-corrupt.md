---
title: "Firebird: database file appears corrupt, wrong page type"
description: "O que significam \"database file appears corrupt\" e \"page is of wrong type\" no Firebird, e como extrair os dados do banco danificado."
lang: pt-BR
---

# `database file appears corrupt` / `page ... is of wrong type`

As duas mensagens costumam vir juntas: `database file appears corrupt (...)`, depois `wrong page type` e `page 1234 is of wrong type (expected 5, found 0)`.

## O que significa

Um banco Firebird é um arquivo de páginas de tamanho fixo, cada uma com o seu tipo no primeiro byte: dados, índice, ponteiros, blob, inventário de transações e assim por diante. O motor seguiu uma referência para a página 1234 esperando um tipo e encontrou outro. *found 0* significa que a página está zerada — típico depois de uma queda de energia, de um disco com defeito, de um reparo do sistema de arquivos ou de uma cópia que deu errado. Outro número geralmente indica que a página contém o conteúdo de uma página diferente.

Quais dados se perdem depende de qual página é:

| Tipo de página (esperado) | O que se perde |
|---|---|
| 5, dados | as linhas dessa página |
| 4, ponteiros | para o motor, as páginas de dados da tabela listadas ali; as próprias páginas estão intactas |
| 7, índice | nada: índices são reconstruídos a partir dos dados |
| 8, blob | os valores BLOB guardados nessa página |
| 3, inventário de transações | veja [`cannot find tip page`](../cannot-find-tip-page/) |

{% include steps-pt.md %}

{% include fbrescue-pt.md %}

O `fbrescue scan` lista todas as páginas que não são o que deveriam ser e, por tabela, quantas páginas de dados e registros ele consegue alcançar. Na exportação:

- páginas de dados que as páginas de ponteiros da tabela já não listam são encontradas lendo o arquivo inteiro (cada página de dados registra a sua tabela e a sua posição nela);
- uma página encontrada no lugar de outra é lida de onde está (a partir do Firebird 3 cada página guarda o próprio número);
- páginas de índice danificadas não importam: a exportação lê só os dados.

## Veja também

- [`I/O error during "read" operation`](../io-error-during-read/) — o arquivo termina antes da hora
- [`cannot find tip page (165)`](../cannot-find-tip-page/)
- [Todos os erros](../)
