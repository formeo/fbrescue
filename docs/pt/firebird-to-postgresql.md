---
title: Migrar os dados do Firebird para o PostgreSQL
description: "Leve as tabelas do Firebird para o PostgreSQL: exporte o .fdb para CSV sem servidor Firebird, mapeie os tipos, carregue com \\copy. MySQL e SQL Server também."
lang: pt-BR
---

# Migrar os dados do Firebird para o PostgreSQL

A parte dos dados de uma migração: levar todas as tabelas de um banco Firebird 2.5–5.0 para o PostgreSQL. O [fbrescue](https://github.com/formeo/fbrescue), uma ferramenta gratuita e de código aberto (licença MIT), lê o arquivo `.fdb` diretamente, então o servidor antigo não precisa estar rodando, e o CSV dele é exatamente o que o `COPY` do PostgreSQL espera.

## 1. Exportar

```
fbrescue export antigo.fdb --out dados
```

Você recebe `dados/schema.sql` com as definições das tabelas e um CSV por tabela. Se o banco usa `CHARACTER SET NONE` para texto gravado por um programa Windows, acrescente `--charset WIN1252` (ou a página de código que ele usava).

## 2. Criar as tabelas

O `schema.sql` está em SQL do Firebird. Crie as mesmas tabelas no PostgreSQL com estes tipos:

| Firebird (`schema.sql`) | PostgreSQL |
|---|---|
| `SMALLINT`, `INTEGER`, `BIGINT`, `BOOLEAN`, `DATE`, `TIME`, `TIMESTAMP`, `DOUBLE PRECISION` | os mesmos |
| `NUMERIC(p,s)`, `DECIMAL(p,s)`, `INT128`, `DECFLOAT` | `numeric(p,s)` / `numeric` |
| `FLOAT` | `real` |
| `CHAR(n)` / `VARCHAR(n) CHARACTER SET …` | `char(n)` / `varchar(n)` (um banco UTF-8) |
| `BLOB SUB_TYPE TEXT` | `text` |
| `BLOB SUB_TYPE BINARY` (ou `0`) | `bytea` — o CSV traz hexadecimal: carregue em `text` e depois use `decode(col, 'hex')` |
| `TIMESTAMP WITH TIME ZONE` | `timestamptz` — o CSV mostra a hora local e o nome do fuso |

Os nomes no `schema.sql` estão entre aspas e em maiúsculas (`"CLIENTES"`). Mantenha-os entre aspas no PostgreSQL ou passe-os para minúsculas no schema e nos comandos abaixo. A precisão de `NUMERIC` no `schema.sql` é a máxima para o tamanho de armazenamento (`NUMERIC(18,2)` para uma coluna `NUMERIC(12,2)`); use a precisão original se você a souber.

## 3. Carregar

```
for f in dados/*.csv; do
  t=$(basename "$f" .csv)
  psql -d destino -c "\copy \"$t\" FROM '$f' WITH (FORMAT csv, HEADER true)"
done
```

O formato CSV do PostgreSQL lê um campo vazio como NULL e `""` como string vazia — exatamente como o fbrescue os grava.

## 4. O que recriar

O fbrescue leva tabelas e linhas. Chaves primárias e estrangeiras, índices, generators (sequences), triggers, procedures e views não são exportados: crie-os no PostgreSQL e ajuste cada sequence para depois da maior chave, por exemplo `SELECT setval('clientes_id_seq', (SELECT max(id) FROM clientes));`.

## MySQL, SQL Server, SQLite

Mesmo CSV, mesmos passos: `LOAD DATA INFILE … FIELDS TERMINATED BY ',' OPTIONALLY ENCLOSED BY '"' IGNORE 1 LINES` no MySQL, `BULK INSERT … WITH (FORMAT = 'CSV', FIRSTROW = 2)` no SQL Server, `.import --csv` no SQLite. Confira como cada um lê campos vazios: nem todos os carregam como NULL.

Mais sobre os arquivos CSV: [Exportar um banco Firebird para CSV]({{ '/pt/firebird-to-csv/' | relative_url }}).
