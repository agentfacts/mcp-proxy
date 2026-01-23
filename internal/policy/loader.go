package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentfacts/mcp-proxy/internal/policy/compiler"
	"github.com/rs/zerolog/log"
)

// Loader handles loading policy files and data.
type Loader struct {
	policyDir     string
	dataFile      string
	jsonPolicyDir string
	compiler      *compiler.Compiler
}

// LoaderOption configures the loader.
type LoaderOption func(*Loader)

// WithJSONPolicyDir sets the JSON policy directory.
func WithJSONPolicyDir(dir string) LoaderOption {
	return func(l *Loader) {
		l.jsonPolicyDir = dir
	}
}

// NewLoader creates a new policy loader.
func NewLoader(policyDir, dataFile string, opts ...LoaderOption) *Loader {
	l := &Loader{
		policyDir:     policyDir,
		dataFile:      dataFile,
		jsonPolicyDir: filepath.Join(policyDir, "json"),
		compiler:      compiler.NewCompiler(),
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// LoadPolicies loads all policy files (.rego and compiled .json) from the policy directory.
func (l *Loader) LoadPolicies() (map[string]string, error) {
	modules := make(map[string]string)

	// Load native Rego files first
	regoModules, err := l.loadRegoFiles()
	if err != nil {
		return nil, err
	}
	for k, v := range regoModules {
		modules[k] = v
	}

	// Load and compile JSON policies
	jsonModules, err := l.loadJSONPolicies()
	if err != nil {
		// Log warning but don't fail if JSON policies can't be loaded
		log.Warn().Err(err).Msg("Failed to load JSON policies, continuing with Rego only")
	} else {
		for k, v := range jsonModules {
			// Check for conflicts - Rego takes precedence
			if _, exists := modules[k]; exists {
				log.Warn().Str("file", k).Msg("JSON policy module conflicts with Rego file, Rego takes precedence")
				continue
			}
			modules[k] = v
		}
	}

	log.Info().Int("count", len(modules)).Str("dir", l.policyDir).Msg("Loaded policy modules")

	return modules, nil
}

// loadRegoFiles loads all .rego files from the policy directory.
func (l *Loader) loadRegoFiles() (map[string]string, error) {
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

		content, err := os.ReadFile(filepath.Clean(file))
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", file, err)
		}

		name := filepath.Base(file)
		modules[name] = string(content)

		log.Debug().Str("file", name).Int("bytes", len(content)).Msg("Loaded Rego policy module")
	}

	return modules, nil
}

// loadJSONPolicies loads and compiles all .json policy files.
func (l *Loader) loadJSONPolicies() (map[string]string, error) {
	modules := make(map[string]string)

	// Check if JSON policy directory exists
	if _, err := os.Stat(l.jsonPolicyDir); os.IsNotExist(err) {
		log.Debug().Str("dir", l.jsonPolicyDir).Msg("JSON policy directory does not exist, skipping")
		return modules, nil
	}

	pattern := filepath.Join(l.jsonPolicyDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob JSON policy files: %w", err)
	}

	if len(files) == 0 {
		log.Debug().Str("dir", l.jsonPolicyDir).Msg("No JSON policy files found")
		return modules, nil
	}

	for _, file := range files {
		content, err := os.ReadFile(filepath.Clean(file))
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", file, err)
		}

		var def compiler.PolicyDefinition
		if err := json.Unmarshal(content, &def); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", file, err)
		}

		result, err := l.compiler.Compile(&def)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %s: %w", file, err)
		}

		// Log warnings
		for _, warn := range result.Warnings {
			log.Warn().Str("file", file).Str("warning", warn).Msg("JSON policy compilation warning")
		}

		// Add compiled modules
		for name, content := range result.Modules {
			modules[name] = content
			log.Debug().Str("source", filepath.Base(file)).Str("generated", name).Int("bytes", len(content)).Msg("Compiled JSON policy to Rego")
		}
	}

	log.Info().Int("count", len(files)).Str("dir", l.jsonPolicyDir).Msg("Compiled JSON policies")

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
	return engine.LoadPolicies(ctx, modules)
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
