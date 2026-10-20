---
title: "Как открыть .fdb без установленного Firebird"
description: "Как посмотреть таблицы файла Firebird .fdb или .gdb и выгрузить данные без установленного сервера — бесплатной утилитой fbrescue."
lang: ru
alt: /open-fdb-without-firebird/
---

# Как открыть файл `.fdb` без установленного Firebird

У вас есть файл базы от старой программы — `.fdb`, иногда `.gdb`. Сервера Firebird нет, пароля нет, а может, нет уже и самой программы. Нужно посмотреть, что внутри, или перенести данные туда, где с ними работают сейчас.

[fbrescue](https://github.com/formeo/fbrescue) читает файл напрямую. Устанавливать ничего не нужно: скачайте бинарник для Windows, Linux или macOS на странице [Releases](https://github.com/formeo/fbrescue/releases/latest). Не нужны ни сервер, ни пользователь, ни пароль, и в файл утилита никогда не пишет.

## Посмотреть, что в файле

```
fbrescue info old.fdb      # версия Firebird, размер страницы, диалект
fbrescue tables old.fdb    # CREATE TABLE: таблицы, колонки, типы
fbrescue scan old.fdb      # страницы, таблицы, число строк; повреждения, если есть
```

## Выгрузить данные

```
fbrescue export old.fdb --out data                        # все таблицы в data/<ТАБЛИЦА>.csv
fbrescue export old.fdb --out data --table CUSTOMERS      # только нужные таблицы
fbrescue export old.fdb --out data --format sql           # INSERT'ы для другой базы Firebird
fbrescue export old.fdb --out data --format jsonl         # JSON Lines для скриптов
```

В каждом формате рядом лежит `schema.sql` с описаниями таблиц. Текст записывается в UTF-8. Русские программы часто хранят текст в `CHARACTER SET NONE`. Если выгрузка предупреждает, что значения не в UTF-8, укажите настоящую кодировку: `--charset WIN1251` (или `DOS866`, `KOI8R`).

Что делать с CSV дальше: [открыть в Excel или загрузить в другую программу]({{ '/ru/firebird-to-csv/' | relative_url }}), [перенести данные в PostgreSQL]({{ '/ru/firebird-to-postgresql/' | relative_url }}).

## Какие файлы поддерживаются

Базы **Firebird 2.5, 3.0, 4.0 и 5.0**, с любым расширением. Файлы InterBase и Firebird 1.x (большинство `.gdb` начала 2000-х) записаны в старом формате и не поддерживаются: `fbrescue info` прямо так и скажет, а не будет гадать.

Если файл повреждён — программа перестала его открывать или его скопировали со сбойного диска, — смотрите [восстановление повреждённой базы]({{ '/ru/' | relative_url }}).

Используйте fbrescue для баз, которые принадлежат вам или к которым у вас есть право доступа.
