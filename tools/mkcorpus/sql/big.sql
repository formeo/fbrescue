-- About one record per 4K page, nearly incompressible: 34 000 data pages.
create table big (id integer not null, payload varchar(4000));
commit;
set term ^;
execute block as
  declare i integer = 1; declare s varchar(4000); declare k integer;
begin
  while (i <= 34000) do begin
    s = ''; k = 0;
    while (k < 100) do begin s = s || uuid_to_char(gen_uuid()); k = k + 1; end
    insert into big values (:i, :s);
    i = i + 1;
  end
end^
set term ;^
commit;
quit;
