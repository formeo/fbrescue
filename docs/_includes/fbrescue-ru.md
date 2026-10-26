## Если gbak всё равно не проходит: выгрузите данные через fbrescue

[fbrescue](https://github.com/formeo/fbrescue) — бесплатная утилита с открытым кодом (лицензия MIT) для баз Firebird 2.5–5.0. Она читает файл базы напрямую, без движка Firebird, и выгружает все строки, которые удаётся прочитать. Она не останавливается на первой повреждённой записи или странице и никогда не пишет в файл. Скачайте бинарник для своей системы на странице [Releases](https://github.com/formeo/fbrescue/releases/latest): Windows, Linux, macOS, устанавливать ничего не нужно.

```
fbrescue scan copy.fdb                               # что повреждено, по таблицам
fbrescue export copy.fdb --out rescued --format sql  # schema.sql + INSERT'ы по таблицам
```

Есть также `--format csv` и `--format jsonl`. Строки, которые не удалось прочитать, перечислены в `rescued/_errors.jsonl` со страницей и причиной; если что-то не выгрузилось, код выхода — 2. Загрузить результат в новую базу:

```
isql -ch UTF8 -i rescued/schema.sql new.fdb
isql -ch UTF8 -i rescued/CUSTOMERS.sql new.fdb  # и так для каждой таблицы
```

Нужна помощь — откройте [issue](https://github.com/formeo/fbrescue/issues) и приложите вывод `fbrescue report copy.fdb`. Отчёт описывает повреждения без ваших данных, имена таблиц и колонок в нём заменены номерами.
