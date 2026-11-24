// Test-only Firebird Array API helper. fbrescue itself does not use C or cgo.
//
// fill:    fbrescue-arrays fill arrays.fdb
// restore: fbrescue-arrays restore new.fdb < commands.txt
//
// Restore commands are ROW <id> and PUT <id> <column> <elements...>.
// Text elements are hexadecimal UTF-8; ~ is an empty string. NULL arrays are
// omitted. The test generates these commands from fbrescue's JSON Lines export.
#include <ibase.h>

#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static ISC_STATUS_ARRAY status;

static void fail(const char *message) {
	fprintf(stderr, "fbrescue-arrays: %s\n", message);
	exit(1);
}

static void check(ISC_STATUS code, const char *where) {
	if (code) {
		fprintf(stderr, "fbrescue-arrays: %s failed:\n", where);
		isc_print_status(status);
		exit(1);
	}
}

static long parse_long(const char *s) {
	char *end;
	errno = 0;
	long n = strtol(s, &end, 10);
	if (errno || *end || n < INT_MIN || n > INT_MAX) {
		fail("invalid row id or array element");
	}
	return n;
}

static long long parse_int64(const char *s) {
	char *end;
	errno = 0;
	long long n = strtoll(s, &end, 10);
	if (errno || *end) {
		fail("invalid BIGINT array element");
	}
	return n;
}

static int hex_digit(char c) {
	if (c >= '0' && c <= '9') return c - '0';
	if (c >= 'a' && c <= 'f') return c - 'a' + 10;
	if (c >= 'A' && c <= 'F') return c - 'A' + 10;
	return -1;
}

static size_t decode_text(const char *s, unsigned char *out, size_t capacity) {
	if (!strcmp(s, "~")) return 0;
	size_t n = strlen(s);
	if (n % 2 || n / 2 > capacity) fail("invalid or too long text array element");
	for (size_t i = 0; i < n; i += 2) {
		int hi = hex_digit(s[i]), lo = hex_digit(s[i + 1]);
		if (hi < 0 || lo < 0) fail("invalid hexadecimal text array element");
		out[i / 2] = (unsigned char)(hi << 4 | lo);
	}
	return n / 2;
}

static long element_count(const ISC_ARRAY_DESC *desc) {
	long count = 1;
	for (int i = 0; i < desc->array_desc_dimensions; i++) {
		long lower = desc->array_desc_bounds[i].array_bound_lower;
		long upper = desc->array_desc_bounds[i].array_bound_upper;
		count *= upper - lower + 1;
	}
	return count;
}

static void insert_row(isc_db_handle db, isc_tr_handle tr, long id) {
	char sql[100];
	snprintf(sql, sizeof(sql), "INSERT INTO \"A\" (\"ID\") VALUES (%ld)", id);
	check(isc_dsql_execute_immediate(status, &db, &tr, 0, sql, 3, NULL), "insert row");
}

static void put_values(isc_db_handle db, isc_tr_handle tr, long id,
	const char *column, char **values, int n, int partial) {
	ISC_ARRAY_DESC desc;
	memset(&desc, 0, sizeof(desc));
	check(isc_array_lookup_bounds(status, &db, &tr, "A", column, &desc), "lookup array bounds");
	if (partial) {
		desc.array_desc_bounds[0].array_bound_upper =
			desc.array_desc_bounds[0].array_bound_lower + partial - 1;
	}
	long count = element_count(&desc);
	int element_length = desc.array_desc_length;
	if (desc.array_desc_dtype == 37) element_length += 2; // VARCHAR length prefix
	if (count != n || count < 1 || element_length < 1 || count > LONG_MAX / element_length) {
		fail("wrong number of array elements");
	}
	ISC_LONG size = (ISC_LONG)(count * element_length);
	unsigned char *data = calloc((size_t)size, 1);
	if (!data) fail("out of memory");

	for (long i = 0; i < count; i++) {
		unsigned char *p = data + i * element_length;
		switch (desc.array_desc_dtype) {
		case 7: { // blr_short
			short v = (short)parse_long(values[i]);
			memcpy(p, &v, sizeof(v));
			break;
		}
		case 8: { // blr_long
			ISC_LONG v = (ISC_LONG)parse_long(values[i]);
			memcpy(p, &v, sizeof(v));
			break;
		}
		case 16: { // blr_int64
			ISC_INT64 v = (ISC_INT64)parse_int64(values[i]);
			memcpy(p, &v, sizeof(v));
			break;
		}
		case 37: { // blr_varying; the API adds the stored length prefix
			if (element_length < 2) fail("invalid VARCHAR element length");
			decode_text(values[i], p, (size_t)element_length - 2);
			break;
		}
		case 14: { // blr_text
			memset(p, ' ', (size_t)element_length);
			decode_text(values[i], p, (size_t)element_length);
			break;
		}
		default:
			fprintf(stderr, "fbrescue-arrays: %s has array element dtype %d\n", column, desc.array_desc_dtype);
			fail("unsupported array element type in test helper");
		}
	}

	ISC_QUAD array_id = {0, 0};
	char where[100];
	snprintf(where, sizeof(where), "put array %s for row %ld", column, id);
	check(isc_array_put_slice(status, &db, &tr, &array_id, &desc, data, &size), where);
	free(data);

	isc_stmt_handle stmt = 0;
	check(isc_dsql_allocate_statement(status, &db, &stmt), "allocate update");
	char sql[160];
	snprintf(sql, sizeof(sql), "UPDATE \"A\" SET \"%s\" = ? WHERE \"ID\" = ?", column);
	check(isc_dsql_prepare(status, &tr, &stmt, 0, sql, 3, NULL), "prepare array update");
	XSQLDA *in = malloc(XSQLDA_LENGTH(2));
	if (!in) fail("out of memory");
	memset(in, 0, XSQLDA_LENGTH(2));
	in->version = SQLDA_VERSION1;
	in->sqln = 2;
	check(isc_dsql_describe_bind(status, &stmt, 1, in), "describe array update");
	ISC_SHORT nulls[2] = {0, 0};
	ISC_LONG row_id = (ISC_LONG)id;
	in->sqlvar[0].sqldata = (ISC_SCHAR *)&array_id;
	in->sqlvar[0].sqlind = &nulls[0];
	in->sqlvar[1].sqldata = (ISC_SCHAR *)&row_id;
	in->sqlvar[1].sqlind = &nulls[1];
	check(isc_dsql_execute(status, &tr, &stmt, 1, in), "update array");
	check(isc_dsql_free_statement(status, &stmt, DSQL_drop), "free update");
	free(in);
}

static void fill(isc_db_handle db, isc_tr_handle tr) {
	char *i1[] = {"10", "-20", "0", "2147483647"};
	char *i2[] = {"10", "-20"};
	char *i3[] = {"-2147483648", "2147483647", "0", "1"};
	char *m[] = {"1", "2", "3", "4", "5", "6"};
	char *v1[] = {"6f6e65", "74776f", "7468726565"}; // one, two, three
	char *v3[] = {"~", "612762", "d096"};             // empty, a'b, Cyrillic Zhe
	char *b1[] = {"-9223372036854775808", "9223372036854775807"};
	char *b3[] = {"0", "-1"};
	char *d[] = {"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"};

	insert_row(db, tr, 1);
	put_values(db, tr, 1, "I", i1, 4, 0);
	put_values(db, tr, 1, "M", m, 6, 0);
	put_values(db, tr, 1, "V", v1, 3, 0);
	put_values(db, tr, 1, "B", b1, 2, 0);
	put_values(db, tr, 1, "D", d, 12, 0);
	insert_row(db, tr, 2);
	put_values(db, tr, 2, "I", i2, 2, 2);
	put_values(db, tr, 2, "M", m, 6, 0);
	insert_row(db, tr, 3);
	put_values(db, tr, 3, "I", i3, 4, 0);
	put_values(db, tr, 3, "V", v3, 3, 0);
	put_values(db, tr, 3, "B", b3, 2, 0);
	insert_row(db, tr, 4); // every array is NULL
}

static void restore(isc_db_handle db, isc_tr_handle tr) {
	char line[8192];
	while (fgets(line, sizeof(line), stdin)) {
		char *tokens[130];
		int n = 0;
		for (char *s = strtok(line, " \t\r\n"); s; s = strtok(NULL, " \t\r\n")) {
			if (n == 130) fail("too many values in restore command");
			tokens[n++] = s;
		}
		if (!n) continue;
		if (!strcmp(tokens[0], "ROW") && n == 2) {
			insert_row(db, tr, parse_long(tokens[1]));
		} else if (!strcmp(tokens[0], "PUT") && n >= 4) {
			put_values(db, tr, parse_long(tokens[1]), tokens[2], tokens + 3, n - 3, 0);
		} else {
			fail("invalid restore command");
		}
	}
	if (ferror(stdin)) fail("cannot read restore commands");
}

int main(int argc, char **argv) {
	if (argc != 3 || (strcmp(argv[1], "fill") && strcmp(argv[1], "restore"))) {
		fprintf(stderr, "usage: fbrescue-arrays fill|restore database.fdb\n");
		return 2;
	}
	isc_db_handle db = 0;
	isc_tr_handle tr = 0;
	check(isc_attach_database(status, 0, argv[2], &db, 0, NULL), "attach");
	check(isc_start_transaction(status, &tr, 1, &db, 0, NULL), "start transaction");
	if (!strcmp(argv[1], "fill")) fill(db, tr); else restore(db, tr);
	check(isc_commit_transaction(status, &tr), "commit");
	check(isc_detach_database(status, &db), "detach");
	return 0;
}
