-- Blobs of every storage level, text in several character sets and binary.
-- On 4K pages a level-1 blob holds up to about 4 MB; larger ones use level 2.
-- Content is built in VARCHARs and doubled: appending to a blob in a loop
-- leaves every intermediate blob in the file.
create table bl (
  id integer not null,
  tu blob sub_type text character set utf8,
  tw blob sub_type text character set win1251,
  tn blob sub_type text character set none,
  bin blob sub_type binary
);
commit;
insert into bl (id, tu, tw, tn, bin) values (1, 'Привет, blob 😀', 'Привет', 'plain', x'00FF10');
insert into bl (id) values (2);
insert into bl (id, tu, tw, tn, bin) values (3, '', '', '', x'');
set term ^;
execute block as
  declare v varchar(8000) character set utf8 = '';
  declare w varchar(8000) character set win1251 = '';
  declare x varchar(8000) character set octets = '';
  declare s blob sub_type text character set utf8;
  declare b blob sub_type binary;
  declare k integer = 0;
begin
  while (octet_length(v) < 7900) do begin
    v = v || 'line ' || k || ' Ёжик ' || ascii_char(10);
    w = w || 'строка ' || k || ';';
    x = x || ascii_char(mod(k * 7, 256)) || ascii_char(mod(k, 256));
    k = k + 1;
  end
  -- level 1: about 30 KB, a page of distinct lines then copies of it
  -- (small enough for the 64 KB statements of Firebird 2.5's isql)
  s = cast(v as blob sub_type text character set utf8) || v || v || v;
  b = cast(x as blob sub_type binary) || x || x || x || x || x || x || x || x || x || x || x || x || x || x || x;
  insert into bl (id, tu, tw, bin) values (4, :s, :w, :b);
  -- level 2: over 4 MB
  k = 0;
  while (k < 8) do begin s = s || s; k = k + 1; end
  insert into bl (id, tu) values (5, :s);
end^
set term ;^
commit;
quit;
