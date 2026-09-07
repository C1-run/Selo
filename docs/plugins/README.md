# Plugin Development Guide

Guide for developing and extending OpenCode plugins for C1 Forge.

## Overview

C1 Forge provides an OpenCode plugin that adds safety tools directly to your editor. The plugin is built using the `@opencode-ai/plugin` SDK.

## Plugin Structure

```
.opencode/
├── plugin.ts                    # Plugin entry point
├── tools/
│   ├── c1_forge_compliance_check.ts  # GateChain compliance tool
│   └── c1forge_plugin_tools.ts       # Additional tools
├── package.json                 # Dependencies
└── node_modules/               # Installed packages
```

## Entry Point

The plugin entry point is `.opencode/plugin.ts`:

```typescript
import type { Plugin, Hooks } from "@opencode-ai/plugin";
import { c1_forge_compliance_check } from "./tools/c1_forge_compliance_check";
import {
  c1forge_submit_task,
  c1forge_task_status,
  c1forge_safety_scan,
  c1forge_daemon_status,
} from "./tools/c1forge_plugin_tools";

const server: Plugin = async (): Promise<Hooks> => {
  return {
    tool: {
      c1_forge_compliance_check,
      c1forge_submit_task,
      c1forge_task_status,
      c1forge_safety_scan,
      c1forge_daemon_status,
    },
  };
};

export const id = "c1forge";
export default { id, server };
```

### Key Exports

| Export | Type | Description |
|--------|------|-------------|
| `id` | `string` | Plugin identifier |
| `server` | `Plugin` | Plugin function returning hooks |

---

## Tool Definition

Each tool is defined using the `tool` helper from the SDK:

```typescript
import { tool } from "@opencode-ai/plugin";

const myTool = tool({
  description: "Description of what the tool does",
  args: {
    param1: tool.schema.string().describe("Parameter description"),
    param2: tool.schema.number().describe("Another parameter"),
  },
  async execute(args) {
    // Tool implementation
    return `Result: ${args.param1}`;
  },
});
```

### Tool Schema Types

| Type | Description | Example |
|------|-------------|---------|
| `tool.schema.string()` | String parameter | `"hello"` |
| `tool.schema.number()` | Number parameter | `42` |
| `tool.schema.boolean()` | Boolean parameter | `true` |
| `tool.schema.array()` | Array parameter | `["a", "b"]` |
| `tool.schema.object()` | Object parameter | `{ key: "value" }` |

### Tool Options

```typescript
const myTool = tool({
  description: "Tool description",
  args: {
    // Required parameter
    required: tool.schema.string().describe("Required param"),
    
    // Optional parameter with default
    optional: tool.schema.string().default("default").describe("Optional param"),
    
    // Enum parameter
    mode: tool.schema.enum(["fast", "slow"]).describe("Mode"),
  },
  async execute(args) {
    // args.required is string
    // args.optional is string (with default)
    // args.mode is "fast" | "slow"
    return "result";
  },
});
```

---

## Existing Tools

### 1. Compliance Check

**File:** `.opencode/tools/c1_forge_compliance_check.ts`

**Description:** Run GateChain compliance check on a steps file.

**Parameters:**
- `stepsFile` (string): Path to the steps.json file

**Usage:**
```typescript
c1_forge_compliance_check({ stepsFile: "steps.json" })
```

### 2. Submit Task

**File:** `.opencode/tools/c1forge_plugin_tools.ts`

**Description:** Submit a coding task to C1 Forge.

**Parameters:**
- `goal` (string): Task goal/description
- `repo` (string, optional): Git repository path
- `commands` (string[], optional): Test commands
- `maxMinutes` (number, optional): Maximum minutes
- `forbiddenFiles` (string[], optional): Files agent must not modify

**Usage:**
```typescript
c1forge_submit_task({
  goal: "Fix the login bug",
  commands: ["go test ./..."],
  maxMinutes: 30
})
```

### 3. Task Status

**File:** `.opencode/tools/c1forge_plugin_tools.ts`

**Description:** Check the status and receipt of a C1 Forge task.

**Parameters:**
- `taskId` (string): Task ID (e.g., 'run-task-123')

**Usage:**
```typescript
c1forge_task_status({ taskId: "run-task-123" })
```

### 4. Safety Scan

**File:** `.opencode/tools/c1forge_plugin_tools.ts`

**Description:** View recent safety scan results.

**Parameters:** None

**Usage:**
```typescript
c1forge_safety_scan()
```

### 5. Daemon Status

**File:** `.opencode/tools/c1forge_plugin_tools.ts`

**Description:** Check if the C1 Forge daemon is running.

**Parameters:** None

**Usage:**
```typescript
c1forge_daemon_status()
```

---

## Creating a New Tool

### Step 1: Create Tool File

```typescript
// .opencode/tools/my_new_tool.ts
import { tool } from "@opencode-ai/plugin";
import { execSync } from "child_process";

export const my_new_tool = tool({
  description: "My new tool description",
  args: {
    input: tool.schema.string().describe("Input parameter"),
  },
  async execute(args) {
    // Tool logic here
    const result = execSync(`selo my-command "${args.input}"`, {
      encoding: "utf-8",
    });
    return result;
  },
});
```

### Step 2: Register in Plugin

```typescript
// .opencode/plugin.ts
import { my_new_tool } from "./tools/my_new_tool";

const server: Plugin = async (): Promise<Hooks> => {
  return {
    tool: {
      // ... existing tools
      my_new_tool,
    },
  };
};
```

### Step 3: Test the Tool

1. Restart OpenCode
2. Use the tool in the TUI
3. Verify output

---

## Advanced Patterns

### Executing Shell Commands

```typescript
import { execSync, spawn } from "child_process";

// Synchronous
const output = execSync("selo status", { encoding: "utf-8" });

// Asynchronous
const output = await new Promise((resolve, reject) => {
  exec("selo status", (error, stdout, stderr) => {
    if (error) reject(error);
    else resolve(stdout);
  });
});
```

### Reading Files

```typescript
import { readFileSync, existsSync } from "fs";
import { join } from "path";

const filePath = join(process.cwd(), "receipt.json");
if (existsSync(filePath)) {
  const content = readFileSync(filePath, "utf-8");
  const receipt = JSON.parse(content);
  return receipt;
}
```

### Error Handling

```typescript
export const my_tool = tool({
  description: "Tool with error handling",
  args: {
    input: tool.schema.string().describe("Input"),
  },
  async execute(args) {
    try {
      const result = await doSomething(args.input);
      return { success: true, data: result };
    } catch (error) {
      return { success: false, error: error.message };
    }
  },
});
```

### Returning Structured Data

```typescript
export const my_tool = tool({
  description: "Tool returning structured data",
  args: {},
  async execute() {
    return {
      status: "ok",
      timestamp: new Date().toISOString(),
      data: {
        count: 42,
        items: ["a", "b", "c"],
      },
    };
  },
});
```

---

## Configuration

### Plugin Registration

The plugin is registered in `opencode.json`:

```json
{
  "plugin": ["./.opencode/plugin.ts"]
}
```

### Dependencies

The plugin requires:

```json
{
  "dependencies": {
    "@opencode-ai/plugin": "1.18.16"
  }
}
```

### TypeScript Configuration

The plugin uses TypeScript with ES modules:

```json
{
  "type": "module"
}
```

---

## Testing

### Manual Testing

1. Start OpenCode in the project directory
2. Use each tool in the TUI
3. Verify output matches expectations

### Automated Testing

```typescript
// .opencode/tools/__tests__/my_tool.test.ts
import { describe, it, expect } from "vitest";
import { my_tool } from "../my_tool";

describe("my_tool", () => {
  it("should return expected result", async () => {
    const result = await my_tool.execute({ input: "test" });
    expect(result).toBe("expected");
  });
});
```

---

## Best Practices

### 1. Descriptive Names

```typescript
// Good
c1forge_submit_task
c1forge_task_status

// Bad
submit
status
```

### 2. Clear Descriptions

```typescript
tool({
  description: "Submit a coding task to C1 Forge for safe execution",
  // ...
})
```

### 3. Parameter Validation

```typescript
args: {
  goal: tool.schema.string()
    .describe("Task goal")
    .min(1)
    .max(1000),
}
```

### 4. Error Messages

```typescript
async execute(args) {
  if (!args.goal) {
    throw new Error("Goal is required");
  }
  // ...
}
```

### 5. Consistent Return Types

```typescript
// Always return the same structure
return {
  success: boolean,
  data?: any,
  error?: string,
}
```

---

## Troubleshooting

### Tool Not Found

1. Check `plugin.ts` exports the tool
2. Restart OpenCode
3. Check debug logs: `opencode run --print-logs --log-level DEBUG "test"`

### TypeScript Errors

1. Run `npm install` in `.opencode/`
2. Check `tsconfig.json`
3. Verify imports

### Plugin Not Loading

1. Check `opencode.json` has `"plugin": ["./.opencode/plugin.ts"]`
2. Verify `plugin.ts` exports `id` and `server`
3. Check debug logs for errors

---

## References

- [API Reference](../api/README.md) — Package documentation
- [Configuration](../configuration/README.md) — Config options
- [Architecture](../architecture/README.md) — System design
