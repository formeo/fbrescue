## First: make a copy, and work only on the copy

Stop the Firebird server, or at least make sure nothing is connected to the database. Then copy the file, together with any `.delta` file next to it (an nbackup lock) and the secondary files of a multi-file database. A copy taken while the server writes to the file is inconsistent, and every repair attempt changes the file it runs on. If the disk itself is failing, copy the file to another disk before anything else.

## Then: try Firebird's own tools

These come with every Firebird installation and often get a database back as a whole: indexes, triggers, users and all. On the **copy**:

```
gfix -v -full -n copy.fdb            # validate, report only
gfix -mend -full -ignore copy.fdb    # let gbak skip damaged records
gbak -b -v -ig -g copy.fdb copy.fbk  # back up: no checksums, no GC
gbak -c -v copy.fbk restored.fdb     # restore into a new database
```

Add `-user SYSDBA -password ...` as your server needs. If `gbak -b` completes, the restored database is the result: check the row counts of the important tables. `gfix -mend` throws away the records it cannot read, so compare with the original before you rely on the restored copy.
