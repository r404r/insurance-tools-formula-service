package mysql

import (
	"errors"
	"os"
	"strings"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// This is a source-level migration contract rather than a mock pretending to
// be MySQL. It is runnable without a database service and makes the DDL and
// serialized last-admin transaction protocol explicit. A DSN-backed suite can
// be added later without weakening this portable contract.
func TestCrossDatabaseIntegrityMigrationContract(t *testing.T) {
	source, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("read migration source: %v", err)
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS system_guards",
		"INSERT IGNORE INTO system_guards (name, version) VALUES ('last_admin', 0)",
		"REFERENCES categories(slug) ON UPDATE RESTRICT ON DELETE RESTRICT",
		"fk_formulas_domain_category",
		"fk_lookup_tables_domain_category",
		"UPDATE system_guards SET version = version + 1 WHERE name = 'last_admin'",
		"BeginTx",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("migration/guard contract missing %q", want)
		}
	}
}

func TestClassifyCategoryDeleteForeignKeyViolation(t *testing.T) {
	err := classifyCategoryDeleteError(&driver.MySQLError{Number: 1451, Message: "foreign key"})
	if !errors.Is(err, storepkg.ErrHasContent) {
		t.Fatalf("classified error = %v, want ErrHasContent", err)
	}
}
