package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCrossDatabaseMigrationCIWorkflow is the CI gate for the opt-in,
// DSN-backed PostgreSQL and MySQL migration suites. Unit tests may skip those
// suites on a developer machine, but a pull-request workflow must run both
// database services and provide a fresh and legacy DSN for each driver.
//
// The workflow is parsed as YAML before its job semantics are inspected. That
// avoids a set of brittle line-oriented assertions while making the required
// execution contract explicit to CI maintainers.
func TestCrossDatabaseMigrationCIWorkflow(t *testing.T) {
	workflowDir := filepath.Join("..", "..", "..", ".github", "workflows")
	paths, err := workflowPaths(workflowDir)
	if err != nil {
		t.Fatalf("read CI workflows: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no GitHub Actions workflow found in %s; add a pull-request job that runs PostgreSQL and MySQL DSN migration suites", workflowDir)
	}

	var examined []string
	for _, path := range paths {
		workflow, err := parseWorkflow(path)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		jobs := yamlMappingValue(workflow, "jobs")
		if jobs == nil || jobs.Kind != yaml.MappingNode {
			examined = append(examined, filepath.Base(path)+": no jobs mapping")
			continue
		}
		for i := 0; i+1 < len(jobs.Content); i += 2 {
			jobName, job := jobs.Content[i].Value, jobs.Content[i+1]
			if jobCoversCrossDatabaseMigrationGate(job) {
				return
			}
			examined = append(examined, fmt.Sprintf("%s:%s", filepath.Base(path), jobName))
		}
	}

	t.Fatalf("no CI job provides real postgres+mysql services, all four DSNs (%s), and go test coverage for both DSN-backed store suites; examined %s",
		strings.Join(crossDatabaseDSNVariables, ", "), strings.Join(examined, ", "))
}

var crossDatabaseDSNVariables = []string{
	"POSTGRES_TEST_DSN",
	"POSTGRES_LEGACY_TEST_DSN",
	"MYSQL_TEST_DSN",
	"MYSQL_LEGACY_TEST_DSN",
}

func workflowPaths(workflowDir string) ([]string, error) {
	entries, err := os.ReadDir(workflowDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension == ".yml" || extension == ".yaml" {
			paths = append(paths, filepath.Join(workflowDir, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func parseWorkflow(path string) (*yaml.Node, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow must be a YAML mapping")
	}
	return document.Content[0], nil
}

func jobCoversCrossDatabaseMigrationGate(job *yaml.Node) bool {
	services := yamlMappingValue(job, "services")
	if !hasContainerService(services, "postgres") || !hasContainerService(services, "mysql") {
		return false
	}

	env := yamlMappingValue(job, "env")
	for _, variable := range crossDatabaseDSNVariables {
		value := yamlScalarValue(env, variable)
		if value == "" || !looksLikeDriverDSN(variable, value) {
			return false
		}
	}

	return jobRunsStoreSuite(job, "postgres") && jobRunsStoreSuite(job, "mysql")
}

func hasContainerService(services *yaml.Node, name string) bool {
	service := yamlMappingValue(services, name)
	return service != nil && service.Kind == yaml.MappingNode && yamlScalarValue(service, "image") != ""
}

func looksLikeDriverDSN(variable, value string) bool {
	value = strings.ToLower(value)
	if strings.HasPrefix(variable, "POSTGRES_") {
		return strings.HasPrefix(value, "postgres://") || strings.HasPrefix(value, "postgresql://")
	}
	return strings.Contains(value, "@tcp(")
}

func jobRunsStoreSuite(job *yaml.Node, driver string) bool {
	steps := yamlMappingValue(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return false
	}
	packagePath := "./internal/store/" + driver
	for _, step := range steps.Content {
		run := yamlScalarValue(step, "run")
		if !strings.Contains(run, "go test") {
			continue
		}
		if strings.Contains(run, packagePath) || strings.Contains(run, "./...") {
			return true
		}
	}
	return false
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func yamlScalarValue(mapping *yaml.Node, key string) string {
	value := yamlMappingValue(mapping, key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(value.Value)
}
