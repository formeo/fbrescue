-- Table t2 with 250 deleted rows left on disk: nothing reads t2 after the
-- delete, so the delete stubs are not garbage-collected.
create table t2 (id integer not null, name varchar(40));
commit;
set term ^;
execute block as declare i integer = 1; begin
  while (i <= 1000) do begin insert into t2 values (:i, 'Row ' || :i); i = i + 1; end
end^
set term ;^
commit;
delete from t2 where mod(id, 4) = 0;
commit;
quit;
