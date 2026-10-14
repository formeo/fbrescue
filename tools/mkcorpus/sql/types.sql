-- Every column type v0.2 exports, with edge values, NULLs and text that is
-- awkward for CSV and SQL (quotes, delimiters, non-ASCII). Run with -ch UTF8.
create table types (
  id integer not null primary key,
  sm smallint, i integer, bi bigint,
  n42 numeric(4,2), n92 numeric(9,2), n184 numeric(18,4), dc decimal(10,3),
  fl float, dp double precision,
  d date, t time, ts timestamp,
  c10 char(10), vc varchar(100),
  cu char(5) character set utf8, vu varchar(30) character set utf8,
  vw varchar(30) character set win1251, vf varchar(30) character set unicode_fss,
  oct char(4) character set octets,
  comp computed by (i * 2)
);
commit;

insert into types values (1, 0, 0, 0, 0, 0, 0, 0, 0, 0, date '1858-11-17', time '00:00:00', timestamp '1858-11-17 00:00:00', '', '', '', '', '', '', x'00000000');
insert into types values (2, -32768, -2147483648, -9223372036854775807 - 1, -99.99, -9999999.99, -99999999999999.9999, -9999999.999,
  -3.4e38, -1.7976931348623157e308, date '0001-01-01', time '23:59:59.9999', timestamp '0001-01-01 00:00:00.0001',
  'x', 'quote '' double " comma , semi ; pipe | back \ end', 'ÀéЖ', 'Привет, мир 😀', 'Привет', 'ΩЖ', x'DEADBEEF');
insert into types values (3, 32767, 2147483647, 9223372036854775807, 99.99, 9999999.99, 99999999999999.9999, 9999999.999,
  3.4e38, 1.7976931348623157e308, date '9999-12-31', time '12:34:56.789', timestamp '9999-12-31 23:59:59.9999',
  'full width', '  leading and trailing  ', 'abcde', '日本語テキスト', 'Ёлка', 'ü', x'01020304');
insert into types (id) values (4);
insert into types values (5, 1, -1, 1, 0.01, -0.01, 0.0001, -0.001, 1.5e-38, 1e-300, date '2024-02-29', time '00:00:00.0001', timestamp '2024-02-29 12:00:00',
  'a', 'line', 'a', 'a', 'a', 'a', x'FF00FF00');
set term ^;
execute block as declare k integer = 6; begin
  while (k <= 2000) do begin
    insert into types values (:k, mod(:k, 30000) - 15000, :k * 1000 - 1000000, :k * cast(1000000007 as bigint) * iif(mod(:k, 2) = 0, 1, -1),
      mod(:k, 9999) / 100.0 - 50, :k * 1.23, :k * 12345.6789, :k / 7.0,
      :k / 3.0, :k / 7.0, dateadd(:k day to date '1990-01-01'), dateadd(:k * 37 second to time '00:00:00'),
      dateadd(:k * 997 second to timestamp '2000-01-01 00:00:00'),
      iif(mod(:k, 5) = 0, null, 'c' || :k), 'Row number ' || :k || iif(mod(:k, 3) = 0, ', with comma', ''),
      iif(mod(:k, 7) = 0, null, 'ж' || mod(:k, 10)), 'Строка ' || :k, 'Строка ' || :k, 'Ф' || :k,
      iif(mod(:k, 11) = 0, null, x'0A0B0C0D'));
    k = k + 1;
  end
end^
set term ;^
commit;
quit;
