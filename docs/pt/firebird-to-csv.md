---
title: Exportar um banco Firebird para CSV ou Excel
description: Exporte as tabelas de um banco Firebird (.fdb) para CSV, JSON Lines ou SQL, sem servidor Firebird, e abra o resultado no Excel, LibreOffice ou num script.
lang: pt-BR
---

# Exportar um banco Firebird para CSV ou Excel

O [fbrescue](https://github.com/formeo/fbrescue), uma ferramenta gratuita e de código aberto (licença MIT), grava cada tabela de um banco Firebird 2.5–5.0 no seu próprio arquivo CSV, direto do arquivo do banco: sem servidor, sem driver ODBC, sem senha. Baixe o executável para o seu sistema em [Releases](https://github.com/formeo/fbrescue/releases/latest) e rode:

```
fbrescue export loja.fdb --out dados
```

```
dados/
  schema.sql        comandos CREATE TABLE
  CLIENTES.csv      um arquivo por tabela
  PEDIDOS.csv
  _errors.jsonl     só se alguma linha não pôde ser lida
```

Use `--table NOME` (pode repetir) para exportar só algumas tabelas. Se um servidor estiver usando o arquivo, faça uma cópia antes: um arquivo copiado enquanto o servidor grava nele pode ficar inconsistente.

## Como fica o CSV

- RFC 4180: separado por vírgulas, uma linha de cabeçalho com os nomes das colunas, UTF-8, valores entre aspas quando necessário.
- **NULL é um campo vazio, uma string vazia é `""`**, então os dois não se confundem.
- Números são exatos: `NUMERIC(18,2)` sai como `1234.50` (ponto decimal), nunca como ponto flutuante.
- Datas `2024-02-29`, horas `13:45:00.0000`, timestamps `2024-02-29 13:45:00.0000`; booleanos `true` / `false`.
- BLOBs de texto são gravados por inteiro; valores binários em hexadecimal.
- Texto em `CHARACTER SET NONE`: acrescente `--charset WIN1252` (ou a codificação que o programa usava, como `ISO8859_1`) para obter UTF-8 legível.

## Abrir no Excel

Com dois cliques no CSV, o Excel adivinha a codificação e o separador: no Brasil ele espera ponto e vírgula, então todas as colunas caem numa só, e os acentos quebram. Em vez disso: **Dados → De Texto/CSV**, escolha o arquivo, defina *Origem do Arquivo* como **65001: Unicode (UTF-8)** e *Delimitador* como **Vírgula**, depois **Carregar**. Os números usam ponto decimal: se o Excel os deixar como texto, clique em **Transformar Dados** e, na coluna, use **Alterar Tipo → Usando a Localidade…** com **Inglês (Estados Unidos)**. O LibreOffice Calc pergunta o mesmo na sua janela de importação.

### Com dois cliques: `--excel`

Onde o Excel usa vírgula decimal (Brasil, Rússia, boa parte da Europa), `fbrescue export antigo.fdb --out dados --excel` grava um CSV que abre com dois cliques: BOM UTF-8 para o Excel reconhecer a codificação, `;` entre os campos e vírgula decimal nos números (textos, datas e horas não mudam). Com dois cliques não dá para escolher o tipo da coluna, então duas coisas mudam no caminho: códigos guardados como texto que parecem números (`00123`, CEP, CPF) perdem os zeros à esquerda, e números com mais de 15 dígitos (ids `BIGINT`, `NUMERIC(18)`) perdem precisão. Para essas colunas use a importação acima, com a coluna como *Texto*.

## Outros formatos

- `--format jsonl`: um objeto JSON por linha, números exatos como números JSON — o mais fácil para Python, Node ou `jq`.
- `--format sql`: comandos INSERT e `schema.sql`, para carregar em outro banco Firebird com o `isql`.

Mudando para outro banco: [Firebird para PostgreSQL]({{ '/pt/firebird-to-postgresql/' | relative_url }}). O arquivo não abre, ou a exportação informa erros: [recuperar um banco corrompido]({{ '/pt/' | relative_url }}).
