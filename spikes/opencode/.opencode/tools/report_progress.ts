import { appendFileSync } from "node:fs"
import { tool } from "@opencode-ai/plugin"

export default tool({
  description: "Report deterministic workflow progress to the CodeGym orchestrator.",
  args: {
    step_id: tool.schema.string().describe("Workflow step identifier"),
    label: tool.schema.string().describe("Human-readable progress label"),
  },
  async execute(args) {
    const path = process.env.OPENCODE_SPIKE_TOOL_LOG
    if (!path) throw new Error("OPENCODE_SPIKE_TOOL_LOG is not set")
    appendFileSync(path, JSON.stringify(args) + "\n", { encoding: "utf8" })
    return JSON.stringify({ accepted: true, ...args })
  },
})
