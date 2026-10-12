-- Base table: inserts, then committed deletes and updates.
-- The update scans the table, so the deleted rows are garbage-collected.
create table t (id integer not null, name varchar(40), amount numeric(12,2), d date);
commit;
set term ^;
execute block as declare i integer = 1; begin
  while (i <= 3000) do begin
    insert into t values (:i, 'Customer number ' || :i, :i * 1.5, date '2020-01-01' + mod(:i, 365));
    i = i + 1;
  end
end^
set term ;^
commit;
delete from t where mod(id, 10) = 0;
commit;
update t set name = 'Updated ' || id where mod(id, 7) = 0;
commit;
quit;
