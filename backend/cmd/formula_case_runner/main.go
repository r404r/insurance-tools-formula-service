package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/shopspring/decimal"

	"github.com/r404r/insurance-tools/formula-service/backend/internal/domain"
)

const (
	defaultBaseURL   = "http://127.0.0.1:8080/api/v1"
	defaultUsername  = "admin"
	defaultPassword  = "admin99999"
	defaultFormulaID = "d764dfd9-777a-43e6-956e-8fbcb467db6f"
)

type fixtureFile struct {
	FormulaID   string        `json:"formulaId"`
	FormulaName string        `json:"formulaName"`
	Version     int           `json:"version"`
	OutputNode  string        `json:"outputNode"`
	GeneratedAt string        `json:"generatedAt"`
	CaseCount   int           `json:"caseCount"`
	Cases       []fixtureCase `json:"cases"`
}

type fixtureCase struct {
	ID             string            `json:"id"`
	Inputs         map[string]string `json:"inputs"`
	ExpectedOutput string            `json:"expectedOutput"`
}

type loginResponse struct {
	Token string `json:"token"`
}

type formulaResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type formulaVersionResponse struct {
	Version int                 `json:"version"`
	State   string              `json:"state"`
	Graph   domain.FormulaGraph `json:"graph"`
}

type validateResponse struct {
	Valid  bool `json:"valid"`
	Errors []struct {
		NodeID  string `json:"nodeId"`
		Message string `json:"message"`
	} `json:"errors"`
}

type calculateResponse struct {
	Result          map[string]string `json:"result"`
	ExecutionTimeMs float64           `json:"executionTimeMs"`
}

type runnerConfig struct {
	baseURL   string
	username  string
	password  string
	formulaID string
	version   int
	fixture   string
	generate  bool
}

type caseResult struct {
	ID           string
	Expected     string
	Actual       string
	ServerTimeMs float64
	RoundTripMs  float64
}

func main() {
	cfg := runnerConfig{}
	flag.StringVar(&cfg.baseURL, "base-url", defaultBaseURL, "API base URL")
	flag.StringVar(&cfg.username, "username", defaultUsername, "login username")
	flag.StringVar(&cfg.password, "password", defaultPassword, "login password")
	flag.StringVar(&cfg.formulaID, "formula-id", defaultFormulaID, "formula ID to validate")
	flag.IntVar(&cfg.version, "version", 0, "formula version to validate; default reads fixture or published version")
	flag.StringVar(&cfg.fixture, "fixture", "testdata/api_cases/d764dfd9-777a-43e6-956e-8fbcb467db6f-v8.json", "fixture file path")
	flag.BoolVar(&cfg.generate, "generate", false, "generate fixture cases before validation")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "formula case runner failed: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg runnerConfig) error {
	client := &http.Client{Timeout: 30 * time.Second}

	token, err := login(client, cfg.baseURL, cfg.username, cfg.password)
	if err != nil {
		return err
	}

	formula, err := fetchFormula(client, cfg.baseURL, token, cfg.formulaID)
	if err != nil {
		return err
	}

	var existingFixture *fixtureFile
	if fixture, err := readFixtureIfExists(cfg.fixture); err == nil {
		existingFixture = fixture
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read fixture: %w", err)
	}

	version := cfg.version
	if version == 0 && existingFixture != nil {
		version = existingFixture.Version
	}
	if version == 0 {
		discoveredVersion, err := fetchPublishedVersion(client, cfg.baseURL, token, cfg.formulaID)
		if err != nil {
			return err
		}
		version = discoveredVersion
	}

	versionDetail, err := fetchVersion(client, cfg.baseURL, token, cfg.formulaID, version)
	if err != nil {
		return err
	}

	if err := validateGraph(client, cfg.baseURL, token, versionDetail.Graph); err != nil {
		return err
	}

	if len(versionDetail.Graph.Outputs) != 1 {
		return fmt.Errorf("expected exactly one output node, got %d", len(versionDetail.Graph.Outputs))
	}
	outputNode := versionDetail.Graph.Outputs[0]

	if cfg.generate {
		fixture := generateLifeNetPremiumFixture(formula.Name, cfg.formulaID, version, outputNode)
		if err := writeFixture(cfg.fixture, fixture); err != nil {
			return err
		}
		fmt.Printf("generated fixture: %s (%d cases)\n", cfg.fixture, fixture.CaseCount)
	}

	fixture, err := readFixture(cfg.fixture)
	if err != nil {
		return fmt.Errorf("load fixture: %w", err)
	}

	if fixture.FormulaID != cfg.formulaID {
		return fmt.Errorf("fixture formula ID mismatch: fixture=%s arg=%s", fixture.FormulaID, cfg.formulaID)
	}
	if fixture.Version != version {
		return fmt.Errorf("fixture version mismatch: fixture=v%d current=v%d", fixture.Version, version)
	}
	if fixture.OutputNode != outputNode {
		return fmt.Errorf("fixture output node mismatch: fixture=%s current=%s", fixture.OutputNode, outputNode)
	}

	results, err := executeCases(client, cfg.baseURL, token, fixture)
	if err != nil {
		return err
	}

	return printSummary(formula.Name, fixture, results)
}

func login(client *http.Client, baseURL, username, password string) (string, error) {
	body := map[string]string{
		"username": username,
		"password": password,
	}
	var resp loginResponse
	if err := postJSON(client, baseURL+"/auth/login", "", body, &resp); err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	if resp.Token == "" {
		return "", errors.New("login returned empty token")
	}
	return resp.Token, nil
}

func fetchFormula(client *http.Client, baseURL, token, formulaID string) (*formulaResponse, error) {
	var resp formulaResponse
	if err := getJSON(client, baseURL+"/formulas/"+formulaID, token, &resp); err != nil {
		return nil, fmt.Errorf("fetch formula: %w", err)
	}
	return &resp, nil
}

func fetchPublishedVersion(client *http.Client, baseURL, token, formulaID string) (int, error) {
	var resp struct {
		Versions []formulaVersionResponse `json:"versions"`
	}
	if err := getJSON(client, baseURL+"/formulas/"+formulaID+"/versions", token, &resp); err != nil {
		return 0, fmt.Errorf("fetch versions: %w", err)
	}
	for _, version := range resp.Versions {
		if version.State == "published" {
			return version.Version, nil
		}
	}
	return 0, errors.New("no published version found")
}

func fetchVersion(client *http.Client, baseURL, token, formulaID string, version int) (*formulaVersionResponse, error) {
	var resp formulaVersionResponse
	if err := getJSON(client, fmt.Sprintf("%s/formulas/%s/versions/%d", baseURL, formulaID, version), token, &resp); err != nil {
		return nil, fmt.Errorf("fetch version v%d: %w", version, err)
	}
	return &resp, nil
}

func validateGraph(client *http.Client, baseURL, token string, graph domain.FormulaGraph) error {
	var resp validateResponse
	if err := postJSON(client, baseURL+"/calculate/validate", token, graph, &resp); err != nil {
		return fmt.Errorf("validate graph: %w", err)
	}
	if resp.Valid {
		return nil
	}
	if len(resp.Errors) == 0 {
		return errors.New("graph validation failed without details")
	}
	return fmt.Errorf("graph validation failed: node=%s message=%s", resp.Errors[0].NodeID, resp.Errors[0].Message)
}

func generateLifeNetPremiumFixture(formulaName, formulaID string, version int, outputNode string) *fixtureFile {
	sumAssureds := []string{"100000", "250000", "500000", "1000000", "5000000"}
	qxs := []string{"0.0001", "0.0005", "0.001", "0.0025", "0.01"}
	interestRates := []string{"0", "0.02", "0.035", "0.05"}

	cases := make([]fixtureCase, 0, len(sumAssureds)*len(qxs)*len(interestRates))
	index := 1
	for _, sumAssured := range sumAssureds {
		for _, qx := range qxs {
			for _, interestRate := range interestRates {
				inputs := map[string]string{
					"sumAssured":   sumAssured,
					"qx":           qx,
					"interestRate": interestRate,
				}
				cases = append(cases, fixtureCase{
					ID:             fmt.Sprintf("life-net-premium-%03d", index),
					Inputs:         inputs,
					ExpectedOutput: calcLifeNetPremiumExpected(inputs),
				})
				index++
			}
		}
	}

	return &fixtureFile{
		FormulaID:   formulaID,
		FormulaName: formulaName,
		Version:     version,
		OutputNode:  outputNode,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CaseCount:   len(cases),
		Cases:       cases,
	}
}

func calcLifeNetPremiumExpected(inputs map[string]string) string {
	sumAssured := mustDecimal(inputs["sumAssured"])
	qx := mustDecimal(inputs["qx"])
	interestRate := mustDecimal(inputs["interestRate"])

	numerator := sumAssured.Mul(qx)
	denominator := decimal.NewFromInt(1).Add(interestRate)
	result := numerator.DivRound(denominator, 28).Round(9)
	return result.String()
}

func mustDecimal(value string) decimal.Decimal {
	d, err := decimal.NewFromString(value)
	if err != nil {
		panic(err)
	}
	return d
}

func executeCases(client *http.Client, baseURL, token string, fixture *fixtureFile) ([]caseResult, error) {
	results := make([]caseResult, 0, len(fixture.Cases))
	for _, tc := range fixture.Cases {
		body := map[string]any{
			"formulaId": fixture.FormulaID,
			"version":   fixture.Version,
			"inputs":    tc.Inputs,
		}

		start := time.Now()
		var resp calculateResponse
		if err := postJSON(client, baseURL+"/calculate", token, body, &resp); err != nil {
			return nil, fmt.Errorf("execute case %s: %w", tc.ID, err)
		}
		roundTripMs := float64(time.Since(start).Microseconds()) / 1000.0

		actual, ok := resp.Result[fixture.OutputNode]
		if !ok {
			return nil, fmt.Errorf("case %s: output node %s missing in response", tc.ID, fixture.OutputNode)
		}
		if actual != tc.ExpectedOutput {
			return nil, fmt.Errorf("case %s: expected %s, got %s", tc.ID, tc.ExpectedOutput, actual)
		}

		results = append(results, caseResult{
			ID:           tc.ID,
			Expected:     tc.ExpectedOutput,
			Actual:       actual,
			ServerTimeMs: resp.ExecutionTimeMs,
			RoundTripMs:  roundTripMs,
		})
	}
	return results, nil
}

func printSummary(formulaName string, fixture *fixtureFile, results []caseResult) error {
	if len(results) == 0 {
		return errors.New("no case results collected")
	}

	var totalServerMs float64
	var totalRoundTripMs float64
	for _, result := range results {
		totalServerMs += result.ServerTimeMs
		totalRoundTripMs += result.RoundTripMs
	}

	avgServerMs := totalServerMs / float64(len(results))
	avgRoundTripMs := totalRoundTripMs / float64(len(results))

	fmt.Printf("formula: %s\n", formulaName)
	fmt.Printf("formulaId: %s\n", fixture.FormulaID)
	fmt.Printf("version: v%d\n", fixture.Version)
	fmt.Printf("outputNode: %s\n", fixture.OutputNode)
	fmt.Printf("cases: %d\n", len(results))
	fmt.Printf("graph validation: PASS\n")
	fmt.Printf("case verification: PASS\n")
	fmt.Printf("average api executionTimeMs: %.3f\n", avgServerMs)
	fmt.Printf("average http roundTripMs: %.3f\n", avgRoundTripMs)
	return nil
}

func readFixtureIfExists(path string) (*fixtureFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fixture fixtureFile
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, err
	}
	return &fixture, nil
}

func readFixture(path string) (*fixtureFile, error) {
	fixture, err := readFixtureIfExists(path)
	if err != nil {
		return nil, err
	}
	return fixture, nil
}

func writeFixture(path string, fixture *fixtureFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create fixture directory: %w", err)
	}
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal fixture: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write fixture: %w", err)
	}
	return nil
}

func getJSON(client *http.Client, url, token string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doJSON(client, req, out)
}

func postJSON(client *http.Client, url, token string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doJSON(client, req, out)
}

func doJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
