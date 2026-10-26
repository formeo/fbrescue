---
title: "Ошибка Firebird: wrong record length (183) — что делать"
description: "Ошибка Firebird «wrong record length (183)»: что она значит и как сохранить данные таблицы, если gfix и gbak не проходят запись."
lang: ru
---

# `internal Firebird consistency check (wrong record length (183))`

## Что это значит

Каждая запись хранит номер своего формата, то есть раскладки таблицы на момент записи, и распаковывается ровно в длину этого формата. `wrong record length` значит, что запись распаковалась в другую длину. Повреждены её байты, заголовок или один из фрагментов. Как и [`decompression overran buffer`](../decompression-overran-buffer/), это повреждение отдельных записей, и оператор, который читает такую запись, падает каждый раз.

{% include steps-ru.md %}

{% include fbrescue-ru.md %}

При этой ошибке fbrescue читает каждую запись в том формате, который она указывает. Запись, которая короче своего формата, попадает в `_errors.jsonl` со страницей и строкой. Остальные записи таблицы выгружаются. Таблицы, которые много раз меняли через ALTER, не проблема: каждая запись читается в той раскладке, с которой её записали.

## См. также

- [`decompression overran buffer (179)`](../decompression-overran-buffer/)
- [`database file appears corrupt`](../database-file-appears-corrupt/)
- [Все ошибки](../)
