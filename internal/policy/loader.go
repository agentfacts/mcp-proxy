package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// Loader handles loading policy files and data.
type Loader struct {
	policyDir string
	dataFile  string
}

// NewLoader creates a new policy loader.
func NewLoader(policyDir, dataFile string) *Loader {
	return &Loader{
		policyDir: policyDir,
		dataFile:  dataFile,
	}
}

// LoadPolicies loads all .rego files from the policy directory.
func (l *Loader) LoadPolicies() (map[string]string, error) {
	modules := make(map[string]string)

	// Find all .rego files
	pattern := filepath.Join(l.policyDir, "*.rego")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob policy files: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .rego files found in %s", l.policyDir)
	}

	for _, file := range files {
		// Skip test files
		if strings.HasSuffix(file, "_test.rego") {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", file, err)
		}

		name := filepath.Base(file)
		modules[name] = string(content)

		log.Debug().Str("file", name).Int("bytes", len(content)).Msg("Loaded policy module")
	}

	log.Info().Int("count", len(modules)).Str("dir", l.policyDir).Msg("Loaded policy modules")

	return modules, nil
}

// LoadPolicyData loads policy data from the JSON file.
func (l *Loader) LoadPolicyData() (map[string]interface{}, error) {
	content, err := os.ReadFile(l.dataFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy data: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse policy data: %w", err)
	}

	log.Info().Str("file", l.dataFile).Int("keys", len(data)).Msg("Loaded policy data")

	return data, nil
}

// LoadAndInitialize loads policies and data, then initializes the engine.
func (l *Loader) LoadAndInitialize(ctx context.Context, engine *Engine) error {
	// Load policy modules
	modules, err := l.LoadPolicies()
	if err != nil {
		return fmt.Errorf("failed to load policies: %w", err)
	}

	// Load policy data
	data, err := l.LoadPolicyData()
	if err != nil {
		return fmt.Errorf("failed to load policy data: %w", err)
	}

	// Set policy data first (so it's available during compilation)
	if err := engine.SetPolicyData(data); err != nil {
		return fmt.Errorf("failed to set policy data: %w", err)
	}

	// Compile policies (with data already set)
	if err := engine.LoadPolicies(ctx, modules); err != nil {
		return fmt.Errorf("failed to compile policies: %w", err)
	}

	return nil
}

// WatchForChanges monitors policy files for changes (placeholder for future implementation).
func (l *Loader) WatchForChanges(ctx context.Context, engine *Engine, onChange func()) error {
	// TODO: Implement file watching with fsnotify
	// For now, this is a placeholder that could be called to set up file watching
	log.Info().Msg("Policy file watching not yet implemented")
	return nil
}

// ValidatePolicies checks if policies can be loaded and compiled without errors.
func (l *Loader) ValidatePolicies(ctx context.Context) error {
	modules, err := l.LoadPolicies()
	if err != nil {
		return err
	}

	// Try to compile with a temporary engine
	engine := NewEngine(EngineConfig{Enabled: true})
	if err := engine.LoadPolicies(ctx, modules); err != nil {
		return err
	}

	return nil
}

// PolicyDataFromStruct converts a PolicyData struct to a map for OPA.
func PolicyDataFromStruct(pd *PolicyData) (map[string]interface{}, error) {
	data, err := json.Marshal(pd)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// LoadPolicyDataStruct loads policy data as a typed struct.
func (l *Loader) LoadPolicyDataStruct() (*PolicyData, error) {
	content, err := os.ReadFile(l.dataFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy data: %w", err)
	}

	var data PolicyData
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse policy data: %w", err)
	}

	return &data, nil
}
