package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/memory"
)

// generationMaxCostMicros is the preflight guard for a one-case generation
// smoke run. It keeps a bounded baseline run far below the $30 milestone.
const generationMaxCostMicros int64 = 1_000_000

// estimateGenerationCostMicros converts provider token counts into a
// conservative planning cost at $2 per million tokens. It is used to settle a
// reserved maximum when the provider does not report a benchmark cost.
func estimateGenerationCostMicros(usage generation.GenerateResult) int64 {
	return int64(usage.TokensIn+usage.TokensOut) * 2
}

// emptyMemory gives the generation orchestrator a valid but neutral profile.
// A benchmark baseline must not inherit a developer's personal memory.
type emptyMemory struct{}

func (emptyMemory) GetProfile(context.Context) (memory.Profile, error) {
	return memory.Profile{}, nil
}

// generationBaseline holds the model-backed harness used to produce a fresh
// reference solution for a saved corpus case. This is a deliberately narrow
// first generation baseline: it exercises real model generation while reusing
// the case's reviewed tests and wrong solutions for assessment.
type generationBaseline struct {
	orchestrator *generation.Orchestrator
	provider     string
	model        string
}

func newGenerationBaseline(providerName string) (*generationBaseline, error) {
	cfg := config.Load()
	providers := cfg.GenAIProviders
	if strings.TrimSpace(providerName) != "" {
		filtered := providers[:0]
		for _, provider := range providers {
			if strings.EqualFold(provider.Name, strings.TrimSpace(providerName)) {
				filtered = append(filtered, provider)
			}
		}
		providers = filtered
		if len(providers) == 0 {
			return nil, fmt.Errorf("generation provider %q is not configured", providerName)
		}
	}
	if len(providers) == 0 {
		return nil, errors.New("no GenAI provider is configured; set META_MUSE_SPARK_API, CODEGYM_GENAI_AZURE_API_KEY, or CODEGYM_GEMINI_API_KEY")
	}
	chosen := providers[0]
	adapter, err := openaicompat.New(openaicompat.Config{
		Name:             chosen.Name,
		BaseURL:          chosen.BaseURL,
		APIKey:           chosen.APIKey,
		Model:            chosen.Model,
		AuthStyle:        chosen.AuthStyle,
		APIVersion:       chosen.APIVersion,
		DefaultMaxTokens: chosen.DefaultMaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("configure generation provider %q: %w", chosen.Name, err)
	}
	return &generationBaseline{
		orchestrator: generation.NewOrchestrator(emptyMemory{}, adapter),
		provider:     chosen.Name,
		model:        chosen.Model,
	}, nil
}

type generatedReference struct {
	Solution string `json:"solution"`
}

// generateReference asks the selected baseline to write one reference solution
// for the case's reviewed spec. It makes exactly one model call per case.
func (b *generationBaseline) generateReference(ctx context.Context, caseDir string) (string, generation.GenerateResult, error) {
	specPath := filepath.Join(caseDir, "spec.json")
	specData, err := os.ReadFile(specPath)
	if err != nil {
		return "", generation.GenerateResult{}, fmt.Errorf("read spec.json: %w", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(specData, &spec); err != nil {
		return "", generation.GenerateResult{}, fmt.Errorf("parse spec.json: %w", err)
	}
	functionName, _ := spec["function_name"].(string)
	language, _ := spec["language"].(string)
	technology, _ := spec["technology"].(string)
	if strings.TrimSpace(language) == "" {
		switch strings.ToLower(strings.TrimSpace(technology)) {
		case "spring-boot", "java":
			language = "java"
		case "express", "node", "nodejs", "javascript":
			language = "javascript"
		case "go", "golang":
			language = "go"
		default:
			language = "python"
		}
	}
	requestSpec, err := json.Marshal(map[string]any{
		"language":      language,
		"technology":    technology,
		"function_name": functionName,
		"spec":          spec,
	})
	if err != nil {
		return "", generation.GenerateResult{}, err
	}
	strategy, _ := spec["strategy"].(string)
	instructions := "You write reference solutions for coding exercises. " +
		"Return one JSON object with a single string field named \"solution\". " +
		"The value must be raw source code only: define the requested function and any helpers it needs, " +
		"with no markdown, no test code, no imports of hidden tests, no shell commands, and no file or network access. " +
		"The code must satisfy every ambiguity resolution in the supplied spec."
	if strings.EqualFold(strings.TrimSpace(strategy), "http") {
		instructions = "You write reference implementations for HTTP coding exercises. " +
			"Return one JSON object with a single string field named \"solution\". " +
			"The value must be raw source code only, with no markdown or test code. " +
			"Implement the exact HTTP method, path, request, and response contract in the supplied spec, including every ambiguity resolution. " +
			"Produce a complete runnable server and also expose the application for tests. " +
			"Do not use shell commands or access the filesystem outside normal framework setup. "
		switch strings.ToLower(language) {
		case "java":
			instructions += "Write Java source for a Spring Boot @RestController in package com.codegym.exercise. " +
				"Use Java double-quoted strings and single quotes only for char literals. " +
				"Return status 400 with JSON {\"error\":\"name is required\"} for missing or blank name " +
				"and status 200 with JSON {\"message\":\"Hello, <name>!\"} otherwise."
		default:
			instructions += "Write JavaScript CommonJS source using require('express'), not ES module import syntax. " +
				"The file is run directly as `node src/app.js` and must start listening immediately. " +
				"Call app.listen(process.env.PORT || 3000) unconditionally at module load. " +
				"Never use `if (require.main === module)` or any other startup guard. " +
				"Set module.exports = app after starting the server."
		}
	} else if strings.EqualFold(strings.TrimSpace(language), "go") {
		instructions += " Write Go source with `package main`. " +
			"Define exactly the function named in the spec with the signature shown there. " +
			"Do not use package solution, package main_test, or any other package name."
	}
	var lastResult generation.GenerateResult
	var lastErr error
	for attempt, maxTokens := range []int{4096, 8192} {
		result, err := b.orchestrator.GenerateWithProfile(ctx, generation.GenerateInput{
			Kind: generation.KindProblem,
			Spec: requestSpec,
			Schema: generation.Schema{
				Name:    "generated_reference",
				Version: "1",
				JSONSchema: json.RawMessage(`{
					"type":"object",
					"required":["solution"],
					"properties":{"solution":{"type":"string"}}
				}`),
			},
			ModelPolicy:  generation.ModelPolicy{MaxTokens: maxTokens},
			Instructions: instructions,
		}, memory.Profile{})
		lastResult = result
		if err != nil {
			lastErr = err
			if attempt == 0 {
				continue
			}
			return "", result, err
		}
		var generated generatedReference
		if err := json.Unmarshal(result.Object, &generated); err != nil {
			lastErr = fmt.Errorf("decode generated reference: %w", err)
			if attempt == 0 {
				continue
			}
			return "", result, lastErr
		}
		if strings.TrimSpace(generated.Solution) == "" {
			lastErr = errors.New("generated reference solution is empty")
			if attempt == 0 {
				continue
			}
			return "", result, lastErr
		}
		return strings.TrimSpace(generated.Solution) + "\n", result, nil
	}
	return "", lastResult, lastErr
}

// materializeGeneratedCase copies a saved case to a temporary directory and
// replaces its reference solution with the model-generated one. The reviewed
// tests, correct alternatives, and wrong solutions are left untouched so the
// existing benchmark assessment measures the generated reference directly.
func materializeGeneratedCase(caseDir, generatedSolution string) (string, error) {
	root, err := os.MkdirTemp("", "pqb-generated-case-")
	if err != nil {
		return "", err
	}
	caseID := filepath.Base(caseDir)
	destination := filepath.Join(root, caseID)
	if err := os.CopyFS(destination, os.DirFS(caseDir)); err != nil {
		return "", fmt.Errorf("copy case %q: %w", caseID, err)
	}
	configPath := filepath.Join(destination, "benchmark-run.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read run config: %w", err)
	}
	var runConfig struct {
		Reference struct {
			Files map[string]string `json:"files"`
		} `json:"reference"`
	}
	if err := json.Unmarshal(data, &runConfig); err != nil {
		return "", fmt.Errorf("parse run config: %w", err)
	}
	if len(runConfig.Reference.Files) == 0 {
		return "", errors.New("run config declares no reference files to replace")
	}
	for _, source := range runConfig.Reference.Files {
		target := filepath.Join(destination, filepath.FromSlash(source))
		if err := os.WriteFile(target, []byte(generatedSolution), 0o640); err != nil {
			return "", fmt.Errorf("write generated reference %q: %w", source, err)
		}
	}
	return destination, nil
}
