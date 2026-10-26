## Se o gbak ainda falhar: extraia os dados com o fbrescue

O [fbrescue](https://github.com/formeo/fbrescue) é uma ferramenta gratuita e de código aberto (licença MIT) para bancos Firebird 2.5–5.0. Ele lê o arquivo do banco diretamente, sem o motor do Firebird, e exporta todas as linhas que consegue ler. Não para no primeiro registro ou página danificada e nunca grava no arquivo. Baixe o executável para o seu sistema em [Releases](https://github.com/formeo/fbrescue/releases/latest): Windows, Linux, macOS; não precisa instalar nada.

```
fbrescue scan copia.fdb                                # o que está danificado, por tabela
fbrescue export copia.fdb --out resgate --format sql   # schema.sql + INSERTs por tabela
```

Também existem `--format csv` e `--format jsonl`. As linhas que não puderam ser lidas ficam listadas em `resgate/_errors.jsonl`, com a página e o motivo, e o código de saída é 2 quando algo ficou de fora. Para carregar o resultado em um banco novo:

```
isql -ch UTF8 -i resgate/schema.sql novo.fdb
isql -ch UTF8 -i resgate/CLIENTES.sql novo.fdb  # e assim por diante, para cada tabela
```

Se o texto sair ilegível, o banco provavelmente guarda texto em `CHARACTER SET NONE`: informe a codificação real, em geral `--charset WIN1252` ou `--charset ISO8859_1` em programas brasileiros.

Se quiser ajuda, abra uma [issue](https://github.com/formeo/fbrescue/issues) e anexe a saída de `fbrescue report copia.fdb`: ela descreve o dano sem nenhum dos seus dados, e com os nomes de tabelas e colunas trocados por números. Pode escrever em português.
