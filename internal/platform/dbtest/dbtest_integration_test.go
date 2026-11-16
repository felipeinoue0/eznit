//go:build integration

package dbtest

import (
	"testing"

	"github.com/jmoiron/sqlx"
)

// The guard is the one thing standing between a mistyped TEST_DATABASE_URL and
// a wiped database, so it is tested as a value, without a connection.
func TestIsTestDatabase(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"eznit_test", true},
		{"eznit_test_backup", false},
		{"project_test", true},
		{"eznit", false},
		{"test", false},
		{"", false},
		{"postgres", false},
		{"eznit_test_prod", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTestDatabase(tt.name); got != tt.want {
				t.Errorf("isTestDatabase(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// The whole schema, not a sample: the account-delete transaction and every
// repository read touch these tables, and a migration that forgot one of them
// would only surface at runtime.
var domainTables = []string{
	"users",
	"sessions",
	"files",
	"files_history",
	"templates",
	"template_files",
	"template_variables",
	"commands",
	"template_commands",
}

// Apply 000001–000014, reverse all of them, apply them again. The down half is
// what makes the migrations history rather than a one-way script, and #156
// found the two most recent ones were only ever verified by hand.
func TestMigrateUpDown(t *testing.T) {
	db := Open(t)

	assertTables(t, db, true)

	MigrateDown(t, db)
	assertTables(t, db, false)

	MigrateUp(t, db)
	assertTables(t, db, true)

	// The columns the later migrations added, named explicitly: a down that
	// dropped a table would hide a column mistake otherwise.
	for _, column := range []struct{ table, name string }{
		{"users", "is_admin"},        // 000013
		{"users", "updated_at"},      // 000005
		{"files", "is_public"},       // 000012
		{"files", "version"},         // 000008
		{"template_files", "kind"},   // 000010
		{"commands", "storage_path"}, // 000011
	} {
		if !columnExists(t, db, column.table, column.name) {
			t.Errorf("%s.%s is missing after re-applying the migrations", column.table, column.name)
		}
	}
}

func assertTables(t *testing.T, db *sqlx.DB, want bool) {
	t.Helper()

	for _, table := range domainTables {
		if got := tableExists(t, db, table); got != want {
			t.Errorf("table %s exists = %v, want %v", table, got, want)
		}
	}
}

func tableExists(t *testing.T, db *sqlx.DB, table string) bool {
	t.Helper()

	var exists bool
	if err := db.Get(&exists, "SELECT to_regclass($1) IS NOT NULL", "public."+table); err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	return exists
}

func columnExists(t *testing.T, db *sqlx.DB, table, column string) bool {
	t.Helper()

	var exists bool
	if err := db.Get(&exists, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
	)`, table, column); err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	return exists
}

// 000016 repairs the rows the old replace path wrote with a dotted extension
// (#358, #361) and leaves a correct row, a dotfile's empty name and a NULL
// alone. It is seeded at 000015, the schema the bad rows were written under.
func TestMigrationStripsTheStoredExtensionDot(t *testing.T) {
	db := Open(t)
	t.Cleanup(func() {
		if err := MigrateUpNow(db); err != nil {
			t.Errorf("restore the full schema: %v", err)
		}
	})

	MigrateUpThrough(t, db, "000015")
	if columnExists(t, db, "files", "ext") == false {
		t.Fatal("files.ext is missing at 000015")
	}

	var owner string
	if err := db.Get(&owner, "INSERT INTO users (email, password) VALUES ('m@example.com', 'x') RETURNING id"); err != nil {
		t.Fatalf("seed a user: %v", err)
	}
	rows := []struct{ id, name, ext, want string }{
		{"00000000-0000-0000-0000-000000000001", "notes", ".txt", "txt"},
		{"00000000-0000-0000-0000-000000000002", "notes", "txt", "txt"},
		{"00000000-0000-0000-0000-000000000003", "archive.tar", "..gz", "gz"},
		{"00000000-0000-0000-0000-000000000004", "Makefile", "", ""},
	}
	for _, row := range rows {
		if _, err := db.Exec("INSERT INTO files (id, user_id, name, ext, path) VALUES ($1, $2, $3, $4, '/x')", row.id, owner, row.name, row.ext); err != nil {
			t.Fatalf("seed %s: %v", row.ext, err)
		}
	}
	if _, err := db.Exec("INSERT INTO files (id, user_id, name, ext, path) VALUES ('00000000-0000-0000-0000-000000000005', $1, 'n', NULL, '/x')", owner); err != nil {
		t.Fatalf("seed a NULL ext: %v", err)
	}

	ApplyMigration(t, db, "000016_strip_file_extension_dot.up.sql")

	for _, row := range rows {
		var got string
		if err := db.Get(&got, "SELECT ext FROM files WHERE id = $1", row.id); err != nil {
			t.Fatalf("read %s: %v", row.id, err)
		}
		if got != row.want {
			t.Errorf("ext %q after 000016 = %q, want %q", row.ext, got, row.want)
		}
	}
	if n := Count(t, db, "files", "ext IS NULL"); n != 1 {
		t.Errorf("NULL ext rows = %d, want 1 (left alone)", n)
	}

	// The down is a no-op and must still run.
	ApplyMigration(t, db, "000016_strip_file_extension_dot.down.sql")
}
