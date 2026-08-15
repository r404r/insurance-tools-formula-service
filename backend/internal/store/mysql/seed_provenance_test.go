package mysql

import storepkg "github.com/r404r/insurance-tools/formula-service/backend/internal/store"

var _ storepkg.SeedResetter = (*MySQLStore)(nil)
