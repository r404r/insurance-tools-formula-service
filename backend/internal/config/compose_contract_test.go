package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeBackendProfilesShareLocalAuthBrowserContract(t *testing.T) {
	composePath := filepath.Join("..", "..", "..", "docker-compose.yml")
	compose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read %s: %v", composePath, err)
	}

	const (
		adminPassword = "${ADMIN_INITIAL_PASSWORD:-admin99999}"
		localCORS     = "http://localhost:3000,http://localhost:5173"
	)

	var jwtSecret string
	for _, service := range []string{"backend-sqlite", "backend-postgres", "backend-mysql"} {
		block := composeServiceBlock(t, string(compose), service)

		if got := composeEnvironment(t, block, "ADMIN_INITIAL_PASSWORD"); got != adminPassword {
			t.Errorf("%s ADMIN_INITIAL_PASSWORD = %q, want %q so bootstrap and seed credentials stay aligned", service, got, adminPassword)
		}
		if got := composeEnvironment(t, block, "SERVER_CORS_ORIGINS"); got != localCORS {
			t.Errorf("%s SERVER_CORS_ORIGINS = %q, want %q so both local frontend ports can use cookie auth", service, got, localCORS)
		}
		if got := composeEnvironment(t, block, "AUTH_COOKIE_SECURE"); got != "false" {
			t.Errorf("%s AUTH_COOKIE_SECURE = %q, want false for local HTTP compose", service, got)
		}

		gotJWT := composeEnvironment(t, block, "AUTH_JWT_SECRET")
		if jwtSecret == "" {
			jwtSecret = gotJWT
		} else if gotJWT != jwtSecret {
			t.Errorf("%s AUTH_JWT_SECRET = %q, want the shared backend value %q", service, gotJWT, jwtSecret)
		}
	}

	seed := composeServiceBlock(t, string(compose), "seed-runner")
	if got := composeEnvironment(t, seed, "SEED_ADMIN_PASS"); got != adminPassword {
		t.Errorf("seed-runner SEED_ADMIN_PASS = %q, want %q to match every backend ADMIN_INITIAL_PASSWORD", got, adminPassword)
	}
}

func composeServiceBlock(t *testing.T, compose, service string) string {
	t.Helper()
	lines := strings.Split(compose, "\n")
	serviceLine := "  " + service + ":"
	for i, line := range lines {
		if line != serviceLine {
			continue
		}

		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "  ") && !strings.HasPrefix(lines[j], "   ") && strings.HasSuffix(lines[j], ":") {
				end = j
				break
			}
		}
		return strings.Join(lines[i+1:end], "\n")
	}
	t.Fatalf("service %q not found in docker-compose.yml", service)
	return ""
}

func composeEnvironment(t *testing.T, serviceBlock, key string) string {
	t.Helper()
	marker := "- " + key + "="
	for _, line := range strings.Split(serviceBlock, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, marker) {
			return strings.TrimPrefix(line, marker)
		}
	}
	return ""
}
