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

// 000017 rebuilds the names the old ReplaceAll split cut short, from the
// stored path (#364). It is seeded at 000016 with what the old code wrote.
func TestMigrationRestoresCutFileNames(t *testing.T) {
	db := Open(t)
	t.Cleanup(func() {
		if err := MigrateUpNow(db); err != nil {
			t.Errorf("restore the full schema: %v", err)
		}
	})

	MigrateUpThrough(t, db, "000016")

	var owner string
	if err := db.Get(&owner, "INSERT INTO users (email, password) VALUES ('n@example.com', 'x') RETURNING id"); err != nil {
		t.Fatalf("seed a user: %v", err)
	}

	const root = "/srv/uploads/" + "11111111-1111-1111-1111-111111111111"
	rows := []struct {
		about             string
		id                string
		name, ext, path   string
		wantName, wantExt string
	}{
		{"extension repeated in the name", "00000000-0000-0000-0000-00000000000a",
			"notes", "txt", root + "/00000000-0000-0000-0000-00000000000a/2_notes.txt.txt", "notes.txt", "txt"},
		{"extension inside the name", "00000000-0000-0000-0000-00000000000b",
			"a-guide", "md", root + "/00000000-0000-0000-0000-00000000000b/1_a.md-guide.md", "a.md-guide", "md"},
		{"already right: untouched", "00000000-0000-0000-0000-00000000000c",
			"report", "pdf", root + "/00000000-0000-0000-0000-00000000000c/1_report.pdf", "report", "pdf"},
		{"a name that starts like a version", "00000000-0000-0000-0000-00000000000d",
			"2024_report", "md", root + "/00000000-0000-0000-0000-00000000000d/3_2024_report.md", "2024_report", "md"},
		{"a dotfile", "00000000-0000-0000-0000-00000000000e",
			"", "bashrc", root + "/00000000-0000-0000-0000-00000000000e/1_.bashrc", "", "bashrc"},
		{"no extension", "00000000-0000-0000-0000-00000000000f",
			"Makefile", "", root + "/00000000-0000-0000-0000-00000000000f/1_Makefile", "Makefile", ""},
		{"Windows separators", "00000000-0000-0000-0000-000000000010",
			"x", "gz", `C:\eznit\uploads\u\00000000-0000-0000-0000-000000000010\1_x.tar.gz`, "x.tar", "gz"},
		// The earliest layout: no file-id folder, the name already cut. Left alone.
		{"legacy layout: untouched", "00000000-0000-0000-0000-000000000011",
			"notes", "txt", "/srv/uploads/1700000000_notes", "notes", "txt"},
		// Another row's id in the path is not this row's file.
		{"someone else's folder: untouched", "00000000-0000-0000-0000-000000000012",
			"keep", "txt", root + "/00000000-0000-0000-0000-00000000000a/2_other.txt", "keep", "txt"},
	}
	for _, row := range rows {
		if _, err := db.Exec("INSERT INTO files (id, user_id, name, ext, path) VALUES ($1, $2, $3, $4, $5)",
			row.id, owner, row.name, row.ext, row.path); err != nil {
			t.Fatalf("seed %s: %v", row.about, err)
		}
	}

	ApplyMigration(t, db, "000017_restore_cut_file_names.up.sql")

	for _, row := range rows {
		var got struct {
			Name string `db:"name"`
			Ext  string `db:"ext"`
		}
		if err := db.Get(&got, "SELECT name, ext FROM files WHERE id = $1", row.id); err != nil {
			t.Fatalf("read %s: %v", row.about, err)
		}
		if got.Name != row.wantName || got.Ext != row.wantExt {
			t.Errorf("%s: (%q, %q), want (%q, %q)", row.about, got.Name, got.Ext, row.wantName, row.wantExt)
		}
	}

	ApplyMigration(t, db, "000017_restore_cut_file_names.down.sql")
}
