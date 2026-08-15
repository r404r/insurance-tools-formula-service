package postgres

import (
	"errors"
	"testing"

	"github.com/lib/pq"
	storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"
)

func TestClassifyUniqueViolation(t *testing.T) {
	err := classifyWriteError(&pq.Error{Code: "23505"})
	if !errors.Is(err, storepkg.ErrConflict) {
		t.Fatalf("classified error = %v, want ErrConflict", err)
	}
}
