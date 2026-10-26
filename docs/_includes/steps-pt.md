## Primeiro: faça uma cópia e trabalhe só na cópia

Pare o servidor Firebird ou, no mínimo, garanta que nada esteja conectado ao banco. Depois copie o arquivo, junto com o arquivo `.delta` ao lado dele, se houver (um bloqueio do nbackup), e os arquivos secundários de um banco multiarquivo. Uma cópia feita enquanto o servidor grava no arquivo fica inconsistente, e toda tentativa de reparo altera o arquivo em que é executada. Se o próprio disco está falhando, copie o arquivo para outro disco antes de qualquer coisa.

## Depois: tente as ferramentas do próprio Firebird

Elas vêm com toda instalação do Firebird e muitas vezes recuperam o banco inteiro: índices, triggers, usuários e tudo mais. Na **cópia**:

```
gfix -v -full -n copia.fdb              # valida, só relata
gfix -mend -full -ignore copia.fdb      # faz o gbak pular registros danificados
gbak -b -v -ig -g copia.fdb copia.fbk   # backup: sem checksums, sem coleta de lixo
gbak -c -v copia.fbk restaurado.fdb     # restaura em um banco novo
```

Acrescente `-user SYSDBA -password ...` conforme o seu servidor exigir. Se o `gbak -b` terminar, o banco restaurado é o resultado: confira a quantidade de linhas das tabelas importantes. O `gfix -mend` descarta os registros que não consegue ler, então compare com o original antes de confiar na cópia restaurada.
