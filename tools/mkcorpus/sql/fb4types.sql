-- Types added in Firebird 4.0. Run with -ch UTF8.
create table t4 (
  id integer not null,
  i128 int128, n38 numeric(38,4), d16 decfloat(16), d34 decfloat(34),
  ttz time with time zone, tstz timestamp with time zone
);
commit;
insert into t4 values (1, 170141183460469231731687303715884105727, 1234567890123456789012345678901234.5678,
  1.234567890123456E+300, -9.999999999999999999999999999999999E+6144,
  time '12:34:56.7890 Europe/Moscow', timestamp '2024-02-29 23:59:59.9999 America/New_York');
insert into t4 values (2, -170141183460469231731687303715884105728, -0.0001,
  -0.000000000000001, 1E-6143,
  time '00:00:00 +05:30', timestamp '0001-01-01 00:00:00 UTC');
insert into t4 (id) values (3);
insert into t4 values (4, 0, 0, 0, 0, time '23:59:59.9999 -12:00', timestamp '9999-12-31 23:59:59.9999 Pacific/Kiritimati');
insert into t4 values (5, 1, 1, 1.5, 12345678901234567890123456789012.34, time '01:00 Asia/Kolkata', timestamp '1970-01-01 00:00:00 GMT');
-- TIME WITH TIME ZONE in zones with daylight saving time: the engine
-- converts with the offset of 2020-01-01.
insert into t4 (id, ttz, tstz) values (6, time '12:00 America/New_York', timestamp '2024-07-01 12:00 America/New_York');
insert into t4 (id, ttz, tstz) values (7, time '12:00 Australia/Sydney', timestamp '2024-07-01 12:00 Australia/Sydney');
-- Coefficients whose 3-digit groups put the digits 8 and 9 in every
-- combination of positions: each case of densely packed decimal.
insert into t4 (id, d16, d34) values (8, 9128182812889999, 1123128182812889898988999181818881);
insert into t4 (id, d16, d34) values (9, -0.9898981818188, 9.818188188818999918881898988889999E-100);
-- ... with small digits 2-7 next to them, whose bits move in each case.
insert into t4 (id, d16, d34) values (10, 2548584854458845, 4828848882884288488238283823999777);
insert into t4 (id, d16, d34) values (11, 7.382832823283382, -6.583538853358835385538355883588353E+6000);
commit;
quit;
