-- Stream blobs (no segment headers): BLOB_APPEND writes them, Firebird 5 only.
set term ^;
execute block as
  declare v varchar(8000) character set utf8 = '';
  declare k integer = 0;
begin
  while (octet_length(v) < 7900) do begin v = v || 'stream ' || k || ' Ёж ' || ascii_char(10); k = k + 1; end
  insert into bl (id, tu, bin) values (6, blob_append(null, 'stream ', 'blob 😀'), blob_append(null, x'0102', x'03'));
  insert into bl (id, tu) values (7, blob_append(null, :v, :v, :v, :v));
  insert into bl (id, tu) values (8, blob_append(null, (select tu from bl where id = 5)));
end^
set term ;^
commit;
quit;
