package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type ProgressReporter func(ctx context.Context, event ProgressEvent) error

type PurposeBuiltRuntime struct {
	Client         CompletionClient
	ReportProgress ProgressReporter
	Now            func() time.Time
}

func (r *PurposeBuiltRuntime) Name() string { return "purpose-built" }

func (r *PurposeBuiltRuntime) Run(ctx context.Context, task TaskSpec) (result RunResult, err error) {
	now := time.Now
	if r != nil && r.Now != nil {
		now = r.Now
	}
	startedAt := now()
	result.Telemetry.Runtime = r.Name()
	result.Telemetry.Termination = TerminationRuntimeFailure
	defer func() {
		result.Telemetry.WallTime = now().Sub(startedAt)
		result.Telemetry.SandboxSeconds = result.Telemetry.WallTime.Seconds()
		result.Manifest = LoadManifest(task.WorkingDirectory)
	}()
	if r == nil || r.Client == nil {
		return result, errors.New("agentruntime: purpose-built runtime client is required")
	}
	if err := task.Validate(startedAt); err != nil {
		return result, err
	}
	if _, err := os.Stat(task.WorkingDirectory); err != nil {
		return result, fmt.Errorf("agentruntime: inspect working directory: %w", err)
	}

	runContext, cancel := context.WithDeadline(ctx, task.Deadline)
	defer cancel()
	root, err := os.OpenRoot(task.WorkingDirectory)
	if err != nil {
		return result, fmt.Errorf("agentruntime: open working directory root: %w", err)
	}
	defer func() { _ = root.Close() }()

	tools := allowedToolDefinitions(task.AllowedTools)
	messages := []ChatMessage{
		{Role: "system", Content: purposeBuiltSystemPrompt(task)},
		{Role: "user", Content: task.Goal},
	}
	for {
		if reason, contextErr := contextTermination(ctx, runContext); reason != "" {
			result.Telemetry.Termination = reason
			return result, contextErr
		}
		if result.Telemetry.Turns >= task.TurnCeiling {
			result.Telemetry.Termination = TerminationTurnCeiling
			return result, nil
		}

		response, completionErr := r.Client.Complete(runContext, CompletionRequest{Messages: messages, Tools: tools})
		result.Telemetry.Turns++
		if completionErr != nil {
			var budgetErr *BudgetError
			if errors.As(completionErr, &budgetErr) {
				result.Telemetry.Termination = budgetErr.Reason
				return result, nil
			}
			if reason, contextErr := contextTermination(ctx, runContext); reason != "" {
				result.Telemetry.Termination = reason
				return result, contextErr
			}
			result.Telemetry.Termination = TerminationRuntimeFailure
			return result, completionErr
		}
		addTokenUsage(&result.Telemetry.Tokens, response.Usage)
		result.Telemetry.CostUSDMicros += response.CostUSD
		if len(response.Message.ToolCalls) == 0 {
			result.Prose = response.Message.Content
			result.Telemetry.Termination = TerminationCompleted
			return result, nil
		}

		// A tool result needs a following model turn to be useful. Refuse every
		// tool from the final permitted model turn so the ceiling is hard.
		if result.Telemetry.Turns >= task.TurnCeiling {
			result.Telemetry.Termination = TerminationTurnCeiling
			return result, nil
		}

		messages = append(messages, response.Message)
		for _, call := range response.Message.ToolCalls {
			if reason, contextErr := contextTermination(ctx, runContext); reason != "" {
				result.Telemetry.Termination = reason
				return result, contextErr
			}
			toolResult, invocation, progress, stopReason := r.executeTool(runContext, root, task, call, now)
			result.Telemetry.ToolInvocations = append(result.Telemetry.ToolInvocations, invocation)
			if stopReason == TerminationRuntimeFailure && strings.HasPrefix(invocation.Error, "attempted disallowed tool") {
				result.Telemetry.PolicyViolations = append(result.Telemetry.PolicyViolations, invocation.Error)
			}
			if progress != nil {
				result.Telemetry.Progress = append(result.Telemetry.Progress, *progress)
			}
			messages = append(messages, ChatMessage{Role: "tool", Content: toolResult, ToolCallID: call.ID})
			if stopReason != "" {
				result.Telemetry.Termination = stopReason
				if stopReason == TerminationOutputCeiling {
					result.Telemetry.OutputTruncated = true
				}
				return result, nil
			}
			if invocation.Tool == ToolShell && invocation.ExitCode != nil && *invocation.ExitCode != 0 {
				result.Telemetry.RepairIterations++
			}
		}
	}
}

func (r *PurposeBuiltRuntime) executeTool(
	ctx context.Context,
	root *os.Root,
	task TaskSpec,
	call ToolCall,
	now func() time.Time,
) (string, ToolInvocation, *ProgressEvent, TerminationReason) {
	tool := Tool(call.Function.Name)
	invocation := ToolInvocation{Tool: tool, StartedAt: now()}
	finish := func(value string, toolErr error, truncated bool) (string, ToolInvocation, *ProgressEvent, TerminationReason) {
		invocation.Duration = now().Sub(invocation.StartedAt)
		invocation.Truncated = truncated
		if toolErr != nil {
			invocation.Error = toolErr.Error()
		}
		payload, _ := json.Marshal(map[string]any{"output": value, "error": invocation.Error, "truncated": truncated})
		stop := TerminationReason("")
		if truncated {
			stop = TerminationOutputCeiling
		}
		return string(payload), invocation, nil, stop
	}
	if !toolAllowed(task.AllowedTools, tool) {
		violation := fmt.Sprintf("attempted disallowed tool %q", call.Function.Name)
		invocation.Error = violation
		invocation.Duration = now().Sub(invocation.StartedAt)
		return `{"error":"tool is not allowed"}`, invocation, nil, TerminationRuntimeFailure
	}

	switch tool {
	case ToolReadFile:
		var input struct {
			Path string `json:"path"`
		}
		if err := decodeToolArguments(call.Function.Arguments, &input); err != nil {
			return finish("", err, false)
		}
		content, truncated, err := readBounded(root, input.Path, task.EffectiveOutputCap())
		return finish(content, err, truncated)
	case ToolWriteFile:
		var input struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := decodeToolArguments(call.Function.Arguments, &input); err != nil {
			return finish("", err, false)
		}
		if len(input.Content) > task.EffectiveOutputCap() {
			return finish("", errors.New("write content exceeds output ceiling"), true)
		}
		err := writeBounded(root, input.Path, []byte(input.Content))
		return finish("file written", err, false)
	case ToolShell:
		var input struct {
			Command string `json:"command"`
		}
		if err := decodeToolArguments(call.Function.Arguments, &input); err != nil {
			return finish("", err, false)
		}
		output, exitCode, truncated, err := runShell(ctx, task.WorkingDirectory, input.Command, task.EffectiveOutputCap())
		invocation.ExitCode = &exitCode
		return finish(output, err, truncated)
	case ToolReportProgress:
		var input struct {
			StepID   string         `json:"step_id"`
			Label    string         `json:"label"`
			Metadata map[string]any `json:"metadata"`
		}
		if err := decodeToolArguments(call.Function.Arguments, &input); err != nil {
			return finish("", err, false)
		}
		event := ProgressEvent{StepID: strings.TrimSpace(input.StepID), Label: strings.TrimSpace(input.Label), Metadata: input.Metadata, At: now()}
		if event.StepID == "" || event.Label == "" {
			return finish("", errors.New("step_id and label are required"), false)
		}
		if r.ReportProgress != nil {
			if err := r.ReportProgress(ctx, event); err != nil {
				return finish("", fmt.Errorf("report progress: %w", err), false)
			}
		}
		invocation.Duration = now().Sub(invocation.StartedAt)
		payload, _ := json.Marshal(map[string]any{"accepted": true, "step_id": event.StepID, "label": event.Label})
		return string(payload), invocation, &event, ""
	default:
		return finish("", fmt.Errorf("unsupported tool %q", tool), false)
	}
}

func purposeBuiltSystemPrompt(task TaskSpec) string {
	return fmt.Sprintf(
		"You are a coding agent operating in one isolated workspace. Use only the supplied tools. Complete the task by editing and testing files. Before your final response, write %s as strict JSON with version=%d, completed, summary, artifacts, and checks. The manifest is an untrusted claim and backend verification decides success. Allowed tools: %s.",
		ManifestRelativePath, ManifestVersion, strings.Join(toolNames(task.AllowedTools), ", "),
	)
}

func allowedToolDefinitions(allowed []Tool) []ToolDefinition {
	definitions := make([]ToolDefinition, 0, len(allowed))
	for _, tool := range allowed {
		definition := ToolDefinition{Type: "function"}
		switch tool {
		case ToolReadFile:
			definition.Function = toolDefinition(string(tool), "Read a bounded file beneath the workspace.", map[string]any{"path": map[string]any{"type": "string"}}, []string{"path"})
		case ToolWriteFile:
			definition.Function = toolDefinition(string(tool), "Write a bounded file beneath the workspace.", map[string]any{
				"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
			}, []string{"path", "content"})
		case ToolShell:
			definition.Function = toolDefinition(string(tool), "Run a shell command in the workspace.", map[string]any{"command": map[string]any{"type": "string"}}, []string{"command"})
		case ToolReportProgress:
			definition.Function = toolDefinition(string(tool), "Report optional detail within the backend-owned workflow step.", map[string]any{
				"step_id": map[string]any{"type": "string"}, "label": map[string]any{"type": "string"},
				"metadata": map[string]any{"type": "object"},
			}, []string{"step_id", "label"})
		}
		definitions = append(definitions, definition)
	}
	return definitions
}

func toolDefinition(name, description string, properties map[string]any, required []string) ToolDefinitionFunction {
	return ToolDefinitionFunction{
		Name: name, Description: description,
		Parameters: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false},
	}
}

func decodeToolArguments(arguments string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return requireJSONEOF(decoder)
}

func toolAllowed(allowed []Tool, target Tool) bool {
	for _, tool := range allowed {
		if tool == target {
			return true
		}
	}
	return false
}

func toolNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for index, tool := range tools {
		names[index] = string(tool)
	}
	return names
}

func addTokenUsage(total *TokenUsage, delta TokenUsage) {
	total.Total += delta.Total
	total.Input += delta.Input
	total.Output += delta.Output
	total.Reasoning += delta.Reasoning
	total.CacheRead += delta.CacheRead
	total.CacheWrite += delta.CacheWrite
}

func contextTermination(parent, run context.Context) (TerminationReason, error) {
	if parent.Err() != nil {
		if errors.Is(parent.Err(), context.DeadlineExceeded) {
			return TerminationDeadline, parent.Err()
		}
		return TerminationCancelled, parent.Err()
	}
	if run.Err() != nil {
		return TerminationDeadline, run.Err()
	}
	return "", nil
}
