-- Record layouts: format changes (ALTER), long runs, records split into
-- fragments across pages, and rows changed by rolled-back transactions.

-- Format changes: rows written under formats 1-4 coexist.
create table alt (id integer not null, a varchar(10), b integer);
commit;
insert into alt values (1, 'one', 10);
insert into alt values (2, null, 20);
commit;
alter table alt add c varchar(20);
commit;
insert into alt values (3, 'three', 30, 'c3');
commit;
alter table alt drop a;
commit;
insert into alt values (4, 40, 'c4');
commit;
alter table alt alter b type bigint;
commit;
insert into alt values (5, 5000000000, 'c5');
commit;
-- Older rows read the default of a NOT NULL column added later.
alter table alt add d integer default 7 not null;
commit;
insert into alt values (6, 60, 'c6', 8);
commit;
-- A nullable column reads NULL in older rows, default or not.
alter table alt add e integer default 9;
alter table alt add f varchar(10) default 'zz' not null;
commit;
insert into alt values (7, 70, 'c7', 9, 10, 'ff');
commit;

-- Blob ids name records by number: rows spread over many data pages.
create table blobs (id integer not null, b blob sub_type text);
commit;
set term ^;
execute block as declare k integer = 1; begin
  while (k <= 3000) do begin
    insert into blobs values (:k, 'blob ' || :k);
    k = k + 1;
  end
end^
set term ;^
commit;

-- Long runs of one byte: the run-length encoding of Firebird 5 differs.
create table runs (id integer not null, c char(3000), v varchar(3000));
commit;
insert into runs values (1, 'x', lpad('', 3000, 'y'));
insert into runs values (2, lpad('a', 3000, 'b'), '');
insert into runs values (3, '', lpad('', 300, 'z') || 'q' || lpad('', 2000, 'z'));
commit;

-- Records larger than a page: stored as a chain of fragments.
create table frag (id integer not null, s varchar(20000));
commit;
set term ^;
execute block as declare k integer = 1; declare s varchar(20000); declare j integer; begin
  while (k <= 20) do begin
    s = ''; j = 0;
    while (j < k * 15) do begin s = s || uuid_to_char(gen_uuid()); j = j + 1; end
    insert into frag values (:k, :s);
    k = k + 1;
  end
end^
set term ;^
commit;

-- Rolled-back changes left on disk: NO AUTO UNDO keeps the record versions
-- of a rolled-back transaction instead of undoing them. Run with
-- housekeeping off so nothing garbage-collects them.
create table rb (id integer not null, v varchar(20));
commit;
insert into rb select rdb$relation_id, 'committed ' || rdb$relation_id from rdb$relations where rdb$relation_id < 20;
commit;
set transaction no auto undo;
insert into rb values (100, 'rolled back insert');
update rb set v = 'rolled back update' where id = 1;
delete from rb where id = 2;
rollback;
quit;
