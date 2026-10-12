-- Enough data to spill past page 300 into the secondary file.
create table x (i integer, s varchar(500));
commit;
set term ^;
execute block as declare i integer = 0; begin
  while (i < 3000) do begin
    insert into x values (:i, lpad('', 400, uuid_to_char(gen_uuid())));
    i = i + 1;
  end
end^
set term ;^
commit;
quit;
