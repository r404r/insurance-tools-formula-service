package auth

import "github.com/r404r/insurance-tools/formula-service/backend/internal/domain"

// Permission represents a granular action that can be performed in the system.
type Permission string

const (
	PermFormulaCreate  Permission = "formula:create"
	PermFormulaEdit    Permission = "formula:edit"
	PermFormulaDelete  Permission = "formula:delete"
	PermFormulaView    Permission = "formula:view"
	PermVersionPublish Permission = "version:publish"
	PermVersionArchive Permission = "version:archive"
	PermCalculate      Permission = "calculate"
	PermUserManage     Permission = "user:manage"
	PermTableManage    Permission = "table:manage"
)

// RolePermissions maps each role to its set of allowed permissions.
var RolePermissions = map[domain.Role][]Permission{
	domain.RoleAdmin: {
		PermFormulaCreate,
		PermFormulaEdit,
		PermFormulaDelete,
		PermFormulaView,
		PermVersionPublish,
		PermVersionArchive,
		PermCalculate,
		PermUserManage,
		PermTableManage,
	},
	domain.RoleEditor: {
		PermFormulaCreate,
		PermFormulaEdit,
		PermFormulaView,
		PermCalculate,
		PermTableManage,
	},
	domain.RoleReviewer: {
		PermFormulaView,
		PermVersionPublish,
		PermVersionArchive,
		PermCalculate,
	},
	domain.RoleViewer: {
		PermFormulaView,
		PermCalculate,
	},
}

// HasPermission checks whether the given role includes the specified permission.
func HasPermission(role domain.Role, perm Permission) bool {
	perms, ok := RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
