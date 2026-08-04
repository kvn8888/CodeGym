# opencode protocol spike findings

Run date: 2026-08-03. Host: Darwin/arm64. Primary evidence is from a successful
fresh isolated HOME at `/tmp/opencode-spike-fresh-3`; the agent process received
an empty inherited environment and no real API credential was read or used.

## 1. Exact request shape

All three primary requests had the same transport shape:

```text
POST /v1/chat/completions
raw query: ""
Authorization: Bearer sk-spike-not-a-real-key
Content-Type: application/json
User-Agent: opencode/1.18.11 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14
```

The recorded headers for the actual agent turn were:

```json
{
  "Accept": ["*/*"],
  "Accept-Encoding": ["gzip, deflate, br, zstd"],
  "Authorization": ["Bearer sk-spike-not-a-real-key"],
  "Connection": ["keep-alive"],
  "Content-Length": ["9764"],
  "Content-Type": ["application/json"],
  "User-Agent": ["opencode/1.18.11 ai-sdk/provider-utils/4.0.23 runtime/bun/1.3.14"],
  "X-Session-Affinity": ["ses_036b65915ffee1iHn1qIVKPf65"],
  "X-Session-Id": ["ses_036b65915ffee1iHn1qIVKPf65"]
}
```

Conclusion: the generic compatible provider uses standard bearer auth, not
Azure's `api-key` header. A CodeGym relay should accept `Authorization: Bearer
<single-operation-token>`.

## 2. Chat Completions or Responses?

The generic provider made three `POST /v1/chat/completions` requests and zero
`/responses` requests. The extra first call generated a session title; the next
two were the tool-call turn and tool-result turn.

The full unabridged request bodies are the `body` fields in
`transcript.jsonl`. The exact protocol-relevant portion of the first agent body
(the large opencode system prompt is shortened here only) was:

```json
{
  "max_tokens": 4096,
  "messages": [
    {"content": "<opencode system prompt; full value in transcript.jsonl>", "role": "system"},
    {"content": "\"Call the report_progress tool with step_id 'environment' and label 'Setting up', then reply DONE.\"", "role": "user"}
  ],
  "model": "spike-model",
  "stream": true,
  "stream_options": {"include_usage": true},
  "tool_choice": "auto",
  "tools": [
    {
      "function": {
        "description": "Report deterministic workflow progress to the CodeGym orchestrator.",
        "name": "report_progress",
        "parameters": {
          "$schema": "https://json-schema.org/draft/2020-12/schema",
          "properties": {
            "label": {"description": "Human-readable progress label", "type": "string"},
            "step_id": {"description": "Workflow step identifier", "type": "string"}
          },
          "required": ["step_id", "label"],
          "type": "object"
        }
      },
      "type": "function"
    }
  ]
}
```

There was no query string. This is the plain OpenAI Chat Completions wire
format already understood by CodeGym's relay direction.

## 3. Tool-call round trip

Yes, all three legs succeeded.

The recorder returned this assistant tool call on the first agent turn:

```json
{
  "role": "assistant",
  "content": null,
  "tool_calls": [{
    "id": "call_opencode_spike_progress",
    "type": "function",
    "function": {
      "name": "report_progress",
      "arguments": "{\"step_id\":\"environment\",\"label\":\"Setting up\"}"
    }
  }]
}
```

The local tool appended exactly one line to `tool-invocations.jsonl`:

```json
{"step_id":"environment","label":"Setting up"}
```

The next request sent both the assistant call and the tool result:

```json
[
  {
    "content": "",
    "role": "assistant",
    "tool_calls": [{
      "function": {
        "arguments": "{\"step_id\":\"environment\",\"label\":\"Setting up\"}",
        "name": "report_progress"
      },
      "id": "call_opencode_spike_progress",
      "type": "function"
    }]
  },
  {
    "content": "{\"accepted\":true,\"step_id\":\"environment\",\"label\":\"Setting up\"}",
    "role": "tool",
    "tool_call_id": "call_opencode_spike_progress"
  }
]
```

The recorder's assertion on that request was:

```json
{"present":true,"shape_valid":true,"expected_tool_call_id":"call_opencode_spike_progress","observed_tool_call_id":"call_opencode_spike_progress","content":"{\"accepted\":true,\"step_id\":\"environment\",\"label\":\"Setting up\"}"}
```

It then returned normal text, and opencode surfaced `DONE — the work is done.`

## 4. Event stream

Distinct `type` values in `events.jsonl`:

```text
step_start   (2)
tool_use    (1)
step_finish  (2)
text         (1)
```

Both `step_finish` events carried token and cost fields. Its relevant fields were:

```json
{"type":"step_finish","part":{"reason":"tool-calls","type":"step-finish","tokens":{"total":48,"input":37,"output":11,"reasoning":0,"cache":{"write":0,"read":0}},"cost":0}}
```

The only `tool_use` event already had `state.status: "completed"` plus both
start and end timestamps (irrelevant IDs omitted below):

```json
{"type":"tool_use","part":{"type":"tool","tool":"report_progress","callID":"call_opencode_spike_progress","state":{"status":"completed","input":{"step_id":"environment","label":"Setting up"},"output":"{\"accepted\":true,\"step_id\":\"environment\",\"label\":\"Setting up\"}","time":{"start":1785788476206,"end":1785788476209}}}}
```

No JSONL event announced the tool at START. Therefore CLI stdout can report
progress after a fast tool completes, but cannot by itself provide a live tool
start signal. A direct callback inside `report_progress` can still stream
immediately to the orchestrator; that path was not tested here.

## 5. Exit code

`exit-code.txt` contains `0`. This was the bounded, unattended success path.

## 6. Egress

The final run started from a path that did not previously exist and copied the
pinned tool runtime into its isolated HOME. `lsof` was sampled every 50 ms for
the opencode command. The only observed remote endpoint was:

```text
127.0.0.1:65006 (TCP, recorder)
```

No non-loopback socket was observed in `egress-lsof.txt`. This is best effort,
not a packet capture: very short-lived sockets and DNS attempts could be missed.

An earlier cold diagnostic showed that opencode bootstraps
`@opencode-ai/plugin` through npm when its config directory has no tool runtime.
The reusable downloader now installs the exact `1.18.11` plugin into ignored
`.bin/home-template` during the explicit network-enabled setup phase. Thus:

- Setup contacts the pinned GitHub release URL and the configured npm registry.
- The actual final opencode run contacted only localhost in the observed data.
- No OpenAI or Azure host was configured or contacted.

## 7. Pinned version and binary

- Version: `1.18.11` (`v1.18.11` release).
- Asset: `https://github.com/sst/opencode/releases/download/v1.18.11/opencode-darwin-arm64.zip`
- Published/verified SHA-256: `188ff6a716bcd40e33ac62f17f4aec9bd760164fa6a2cde66f779a5db4abc7ce`
- Extracted binary size: `138,608,738` bytes.
- Downloaded archive size: `44,962,786` bytes.

Both binary and archive live under ignored `spikes/opencode/.bin/`.

## 8. Primary verdict

**GO-WITH-CAVEATS.** opencode 1.18.11 can be driven against a plain
OpenAI-compatible relay through `@ai-sdk/openai-compatible`, including a real
streaming tool-call execution and correctly shaped tool-result continuation.

Caveats to carry into production design:

1. Preload the pinned custom-tool runtime in the Daytona image; an empty config
   directory otherwise causes npm bootstrap egress.
2. Budget/account for a separate title-generation completion at session start.
3. The CLI's `tool_use` event appears only at completion. Use the custom tool's
   direct backend callback for immediate progress, or separately test serve SSE.
4. Continue to enforce a backend-verified result manifest; exit 0 proves runner
   success, not semantic task success.

## 9. Secondary native Azure verdict

**NO-GO for the tested native configuration.** This lower-priority probe set
`useCompletionUrls: true` in both `provider.azure.options` and the model's
`options`, used dummy `apiKey`, `apiVersion: "2024-10-21"`, and a localhost
deployment base. opencode nevertheless emitted:

```text
POST /openai/deployments/responses
raw query: ""
api-key: sk-spike-not-a-real-key
```

The relevant recorded headers were:

```json
{
  "Api-Key": ["sk-spike-not-a-real-key"],
  "Content-Type": ["application/json"],
  "User-Agent": ["opencode/1.18.11 ai-sdk/provider-utils/4.0.38 runtime/bun/1.3.14"]
}
```

The body was Responses-format (`input`, not `messages`). The probe made two
Responses requests (title plus agent), then exited 1 after the recorder returned
its catch-all JSON 404 for the unexpected deployment-prefixed Responses path.
It did **not** produce the required
deployment-prefixed `.../{deployment}/chat/completions?api-version=...` shape.
This does not block the primary architecture: the generic compatible provider
is the intended sandbox-to-CodeGym boundary, and CodeGym owns Azure translation
server-side.
