-- BOOLEAN exists from Firebird 3.0 on.
alter table types add bool boolean;
commit;
update types set bool = iif(mod(id, 3) = 0, null, mod(id, 2) = 0);
commit;
quit;
