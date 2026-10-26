---
title: "Recuperação de Firebird: gfix/gbak, FirstAID, fbrescue"
description: "gfix/gbak, IBSurgeon FirstAID ou fbrescue, a alternativa gratuita e de código aberto, para um banco Firebird corrompido: o que cada um faz e quanto custa."
lang: pt-BR
---

# Ferramentas de recuperação do Firebird comparadas

Três tipos de ferramenta recuperam dados de um banco Firebird danificado. Elas fazem coisas diferentes e podem ser usadas uma depois da outra: comece pelas ferramentas do próprio motor, numa cópia do arquivo.

| | gfix + gbak | IBSurgeon FirstAID | fbrescue |
|---|---|---|---|
| O que faz | valida e marca registros danificados (`gfix -mend`), depois faz backup e restore através do motor | conserta o arquivo no lugar ("direct fix") ou extrai os dados para um banco novo com a mesma estrutura | exporta os dados (CSV, INSERTs SQL, JSON Lines) e as definições das tabelas, sem o motor |
| Altera o arquivo | o `gfix -mend` altera | o direct fix altera | nunca: só leitura |
| O que você recebe | o banco inteiro: índices, triggers, procedures, usuários | o banco consertado, ou os dados dele | tabelas e linhas; índices, triggers e procedures precisam ser criados de novo |
| Quando para | no primeiro registro ou página que o motor não consegue ler | — | não para: pula e lista o que não consegue ler |
| Versões do Firebird | as do próprio servidor | Firebird 1.0–5.0, InterBase 4.0–2020 | Firebird 2.5–5.0 |
| Plataformas | onde o Firebird roda | Windows 7 ou posterior (copie os arquivos de servidores Linux) | Windows, Linux, macOS; um executável, sem precisar do Firebird |
| Interface | linha de comando | gráfica | linha de comando, saída JSON para scripts |
| Preço | gratuito, faz parte do Firebird | prévia gratuita; uma licença por banco para o reparo (a partir de US$ 199 por 3 bancos)¹ | gratuito, licença MIT |
| Versão gratuita | tudo | mostra os dados; consertar ou extrair exige licença | tudo |
| Código-fonte | aberto | fechado | [aberto](https://github.com/formeo/fbrescue), com [taxa de recuperação publicada](https://github.com/formeo/fbrescue#recovery-rate) |

¹ Segundo a [página do FirstAID da IBSurgeon](https://ib-aid.com/en/ibsurgeon-firstaid/), outubro de 2026. A IBSurgeon também oferece um serviço de recuperação para casos graves.

## Qual usar

- **Sempre primeiro: gfix e gbak**, numa cópia. Se o `gbak -b -ig -g` terminar, o banco restaurado está completo, com tudo o que o motor conhece. Os passos estão na [página de recuperação]({{ '/pt/' | relative_url }}).
- **fbrescue** quando o gbak para, quando você está no Linux ou macOS, quando precisa dos dados de um banco que nem abre, ou para ver rapidamente o que dá para recuperar: o `fbrescue scan` relata o dano tabela por tabela, e o `export` extrai as linhas sem tocar no arquivo. É gratuito, então não há nada a perder testando numa cópia.
- **IBSurgeon FirstAID** quando você precisa do próprio banco consertado no lugar em vez dos dados exportados, para InterBase ou Firebird anterior ao 2.5, ou quando quer uma ferramenta comercial com suporte e um serviço de recuperação por trás.

As ferramentas não se excluem: o fbrescue nunca grava no arquivo, então pode ser usado numa cópia antes ou depois de qualquer outra tentativa.

## Veja também

- [Mensagens de erro e o que significam]({{ '/pt/errors/' | relative_url }})
- [FAQ]({{ '/pt/faq/' | relative_url }})
