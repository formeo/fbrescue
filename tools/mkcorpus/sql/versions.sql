-- Record version chains left on disk. Housekeeping is off and nothing reads
-- the table afterwards, so no version is garbage-collected.
create table vers (id integer not null, v varchar(60), n integer);
commit;
insert into vers values (1, 'one', 1);
insert into vers values (2, 'two', 2);
insert into vers values (3, 'three', 3);
insert into vers values (4, 'four', 4);
insert into vers values (5, 'five', 5);
insert into vers values (7, 'seven', 7);
insert into vers values (8, 'eight', 8);
commit;
-- 1: three committed updates; the newest is current.
update vers set v = 'one a', n = 11 where id = 1;
commit;
update vers set v = 'one b', n = 12 where id = 1;
commit;
update vers set v = 'one c' where id = 1;
commit;
-- 5: deleted and committed: gone, but the old version is still on disk.
delete from vers where id = 5;
commit;
-- 7: updated and committed, then updated again by a rolled-back transaction.
update vers set v = 'seven committed', n = 77 where id = 7;
commit;
-- The rolled-back transaction: NO AUTO UNDO leaves its versions on disk.
set transaction no auto undo;
update vers set v = 'two rolled back', n = 20 where id = 2;
update vers set v = 'three rolled back' where id = 3;
update vers set v = 'three rolled back again', n = 30 where id = 3;
delete from vers where id = 4;
update vers set v = 'seven rolled back' where id = 7;
update vers set v = lpad('', 60, 'x') where id = 8;
insert into vers values (6, 'six never committed', 6);
rollback;
quit;
