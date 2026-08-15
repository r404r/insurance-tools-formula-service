package postgres

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

// These contracts deliberately inspect the migration source because this
// repository does not provision a Postgres service for normal unit tests.
// They keep the portable schema and transaction protocol reviewable in CI;
// a DSN-backed integration suite may exercise the same contracts when one is
// supplied by the environment.
func TestCrossDatabaseIntegrityMigrationContract(t *testing.T) {
	source, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("read migration source: %v", err)
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS system_guards",
		"INSERT INTO system_guards (name, version) VALUES ('last_admin', 0) ON CONFLICT (name) DO NOTHING",
		"REFERENCES categories(slug) ON UPDATE RESTRICT ON DELETE RESTRICT",
		"fk_formulas_domain_category",
		"fk_lookup_tables_domain_category",
		"UPDATE system_guards SET version = version + 1 WHERE name = 'last_admin'",
		"r.db.BeginTx", // guard against accidentally replacing the protocol with non-transactional reads
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("migration/guard contract missing %q", want)
		}
	}
}

func TestClassifyCategoryDeleteForeignKeyViolation(t *testing.T) {
	err := classifyCategoryDeleteError(&pq.Error{Code: "23503"})
	if !errors.Is(err, storepkg.ErrHasContent) {
		t.Fatalf("classified error = %v, want ErrHasContent", err)
	}
}
