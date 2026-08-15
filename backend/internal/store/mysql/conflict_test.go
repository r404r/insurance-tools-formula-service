package mysql

import (
	"errors"
	"testing"

	driver "github.com/go-sql-driver/mysql"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

func TestClassifyUniqueViolation(t *testing.T) {
	err := classifyWriteError(&driver.MySQLError{Number: 1062, Message: "duplicate"})
	if !errors.Is(err, storepkg.ErrConflict) {
		t.Fatalf("classified error = %v, want ErrConflict", err)
	}
}
