-- A small shop: the target of the damage tests. Three tables with primary
-- keys and a foreign key (so there are index pages), text blobs (most on the
-- data pages, some on blob pages), updates and deletes.
create table customers (id integer not null primary key, name varchar(60) not null, city varchar(30), created timestamp);
create table orders (id integer not null primary key, customer_id integer not null references customers,
  amount numeric(12,2), status smallint, note blob sub_type text);
create table items (order_id integer not null references orders, line smallint not null, product varchar(40),
  qty integer, price numeric(10,2), primary key (order_id, line));
commit;
set term ^;
execute block as
  declare i integer = 1;
  declare j integer;
  declare n blob sub_type text;
  declare s varchar(8000);
begin
  while (i <= 2000) do begin
    insert into customers values (:i, 'Customer ' || :i || ' Ltd', 'City ' || mod(:i, 97),
      timestamp '2021-01-01 09:00:00' + :i / 7.0);
    i = i + 1;
  end
  i = 1;
  while (i <= 5000) do begin
    n = null;
    if (mod(:i, 3) = 0) then n = 'Deliver to the back door, order ' || :i;
    if (mod(:i, 250) = 0) then begin
      -- Built as a string: appending to a blob in a loop leaves a
      -- temporary blob behind for every step.
      s = 'Long note ' || :i || ': ';
      j = 0;
      while (j < 120) do begin
        s = s || 'line ' || :j || ' of a note too long for the data page. ';
        j = j + 1;
      end
      n = s;
    end
    insert into orders values (:i, mod(:i * 7, 2000) + 1, :i * 3.25, mod(:i, 4), :n);
    j = 1;
    while (j <= mod(:i, 5) + 1) do begin
      insert into items values (:i, :j, 'Product ' || mod(:i * :j, 300), :j * 2, mod(:i, 90) + 0.99);
      j = j + 1;
    end
    i = i + 1;
  end
end^
set term ;^
commit;
update orders set status = 9 where mod(id, 11) = 0;
delete from items where mod(order_id, 13) = 0 and line > 1;
commit;
quit;
