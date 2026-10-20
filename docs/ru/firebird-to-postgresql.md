---
title: Перенос данных из Firebird в PostgreSQL
description: "Перенос таблиц Firebird в PostgreSQL: выгрузка .fdb в CSV без сервера, соответствие типов, загрузка через \\copy. Также MySQL и SQL Server."
lang: ru
alt: /firebird-to-postgresql/
---

# Перенос данных из Firebird в PostgreSQL

Эта страница — про перенос данных при миграции: как перенести все таблицы базы Firebird 2.5–5.0 в PostgreSQL. [fbrescue](https://github.com/formeo/fbrescue) читает файл `.fdb` напрямую, поэтому старый сервер запускать не нужно, а его CSV — именно то, что ждёт `COPY` в PostgreSQL.

## 1. Выгрузка

```
fbrescue export old.fdb --out data
```

Получится `data/schema.sql` с описаниями таблиц и по CSV на таблицу. Если программа под Windows хранила текст в `CHARACTER SET NONE`, добавьте `--charset WIN1251` (или кодовую страницу, с которой она работала).

## 2. Создать таблицы

`schema.sql` написан на SQL Firebird. Создайте в PostgreSQL те же таблицы с такими типами:

| Firebird (`schema.sql`) | PostgreSQL |
|---|---|
| `SMALLINT`, `INTEGER`, `BIGINT`, `BOOLEAN`, `DATE`, `TIME`, `TIMESTAMP`, `DOUBLE PRECISION` | те же |
| `NUMERIC(p,s)`, `DECIMAL(p,s)`, `INT128`, `DECFLOAT` | `numeric(p,s)` / `numeric` |
| `FLOAT` | `real` |
| `CHAR(n)` / `VARCHAR(n) CHARACTER SET …` | `char(n)` / `varchar(n)` (база в UTF-8) |
| `BLOB SUB_TYPE TEXT` | `text` |
| `BLOB SUB_TYPE BINARY` (или `0`) | `bytea` — в CSV шестнадцатеричный вид: загрузите в `text`, затем `decode(col, 'hex')` |
| `TIMESTAMP WITH TIME ZONE` | `timestamptz` — в CSV местное время и имя пояса |

Имена в `schema.sql` записаны прописными буквами в кавычках (`"CUSTOMERS"`). Либо оставьте их в кавычках и в PostgreSQL, либо переведите в нижний регистр — и в схеме, и в командах ниже. Точность `NUMERIC` в `schema.sql` максимальная для размера хранения: `NUMERIC(18,2)` вместо `NUMERIC(12,2)`. Если знаете исходную точность, используйте её.

## 3. Загрузка

```
for f in data/*.csv; do
  t=$(basename "$f" .csv)
  psql -d target -c "\copy \"$t\" FROM '$f' WITH (FORMAT csv, HEADER true)"
done
```

Формат CSV в PostgreSQL читает пустое поле как NULL, а `""` — как пустую строку. fbrescue пишет ровно так же.

## 4. Что создать заново

fbrescue переносит таблицы и строки. Первичные и внешние ключи, индексы, генераторы (последовательности), триггеры, процедуры и представления не выгружаются: создайте их в PostgreSQL. Каждую последовательность выставьте за максимальный ключ, например `SELECT setval('customers_id_seq', (SELECT max(id) FROM customers));`.

## MySQL, SQL Server, SQLite

Тот же CSV и те же шаги:
- MySQL: `LOAD DATA INFILE … FIELDS TERMINATED BY ',' OPTIONALLY ENCLOSED BY '"' IGNORE 1 LINES`;
- SQL Server: `BULK INSERT … WITH (FORMAT = 'CSV', FIRSTROW = 2)`;
- SQLite: `.import --csv`.

Проверьте, как каждая СУБД читает пустые поля: не все загружают их как NULL.

Подробнее о CSV — [выгрузка базы Firebird в CSV]({{ '/ru/firebird-to-csv/' | relative_url }}).
