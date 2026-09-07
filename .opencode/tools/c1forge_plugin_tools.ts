import { tool } from "@opencode-ai/plugin";
import { z } from "zod";
import { dirname, resolve } from "node:path";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { execFile } from "node:child_process";

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");

function resolveBinary(): string {
  const repoBin = resolve(REPO_ROOT, "selo");
  if (existsSync(repoBin)) {
    return repoBin;
  }
  return "selo";
}

function execPromise(bin: string, args: string[]): Promise<{ exitCode: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    execFile(
      bin,
      args,
      { encoding: "utf8", maxBuffer: 10 * 1024 * 1024 },
      (err, stdout, stderr) => {
        const code = err ? (typeof (err as { code?: unknown }).code === "number" ? ((err as { code: number }).code) : 1) : 0;
        resolve({ exitCode: code, stdout: stdout ?? "", stderr: stderr ?? "" });
      },
    );
  });
}

export const c1forge_submit_task = tool({
  description:
    "Submit a coding task to C1 Forge for safe execution with full audit trail. " +
    "Creates a task in the queue, processes it through the safety pipeline (governor, " +
    "scans, Pinocchio, GateChain), and produces a receipt. The task will be picked up " +
    "by the next daemon cycle or processed immediately with `selo run`.",
  args: {
    goal: z.string().describe("The task goal/description for the coding agent"),
    repo: z.string().optional().describe("Git repository path (default: current project root)"),
    commands: z.array(z.string()).optional().describe("Test commands to run after the agent completes"),
    maxMinutes: z.number().optional().describe("Maximum minutes for the agent (default: 30)"),
    forbiddenFiles: z.array(z.string()).optional().describe("Files the agent must not modify"),
  },
  execute: async (args, context) => {
    const repoPath = args.repo ? resolve(context.directory, args.repo) : context.directory;

    // Write task.md to the pending queue
    const pendingDir = resolve(context.directory, ".selo", "queue", "pending");
    const taskId = `plugin-${Date.now()}`;
    const taskPath = resolve(pendingDir, `${taskId}.md`);

    let taskContent = `id: "${taskId}"
goal: "${args.goal}"
repo: "${repoPath}"
`;
    if (args.commands && args.commands.length > 0) {
      taskContent += "commands:\n";
      for (const cmd of args.commands) {
        taskContent += `  - "${cmd}"\n`;
      }
    }
    if (args.maxMinutes) {
      taskContent += `max_minutes: ${args.maxMinutes}\n`;
    }
    if (args.forbiddenFiles && args.forbiddenFiles.length > 0) {
      taskContent += "forbidden_files:\n";
      for (const f of args.forbiddenFiles) {
        taskContent += `  - "${f}"\n`;
      }
    }

    const { writeFileSync, mkdirSync } = await import("node:fs");
    mkdirSync(pendingDir, { recursive: true });
    writeFileSync(taskPath, taskContent, "utf8");

    return {
      title: "Task submitted to C1 Forge",
      output: `Task ${taskId} queued for processing.\nGoal: ${args.goal}\nRepository: ${repoPath}\n\nThe daemon will pick this up on the next poll cycle, or you can process it immediately with:\nselo daemon --one-shot`,
      metadata: {
        task_id: taskId,
        task_path: taskPath,
        repo: repoPath,
        goal: args.goal,
      },
    };
  },
});

export const c1forge_task_status = tool({
  description:
    "Check the status and receipt of a C1 Forge task. Returns the verdict, timing, " +
    "safety hits, and diff summary from the task's receipt.json.",
  args: {
    taskId: z.string().describe("Task ID to check (e.g., 'run-task-123')"),
  },
  execute: async (args, context) => {
    const receiptPath = resolve(context.directory, ".selo", "runs", `run-${args.taskId}`, "receipt.json");

    if (!existsSync(receiptPath)) {
      // Try pending queue
      const pendingPath = resolve(context.directory, ".selo", "queue", "pending", `${args.taskId}.md`);
      if (existsSync(pendingPath)) {
        return {
          title: "Task status: PENDING",
          output: `Task ${args.taskId} is in the pending queue.\nPath: ${pendingPath}`,
          metadata: { status: "pending", task_id: args.taskId },
        };
      }
      return `error: no receipt or task found for ${args.taskId}`;
    }

    try {
      const data = readFileSync(receiptPath, "utf8");
      const receipt = JSON.parse(data);

      const lines: string[] = [];
      lines.push(`Task: ${receipt.task_id}`);
      lines.push(`Verdict: ${receipt.final_verdict || receipt.verdict}`);
      lines.push(`Duration: ${receipt.duration_sec}s`);
      lines.push(`Files changed: ${receipt.files_changed}`);
      lines.push(`Patch lines: ${receipt.patch_lines}`);
      lines.push(`Scans passed: ${receipt.scans_passed}`);
      lines.push(`Test integrity: ${receipt.test_integrity_passed ? "passed" : "FAILED"}`);

      if (receipt.safety_hits && receipt.safety_hits.length > 0) {
        lines.push(`Safety hits: ${receipt.safety_hits.join("; ")}`);
      }
      if (receipt.pinocchio_false_claims && receipt.pinocchio_false_claims.length > 0) {
        lines.push(`Pinocchio false claims: ${receipt.pinocchio_false_claims.join("; ")}`);
      }
      if (receipt.diff_summary) {
        lines.push(`\nDiff summary:\n${receipt.diff_summary}`);
      }

      return {
        title: `Task ${args.taskId}: ${receipt.final_verdict || receipt.verdict}`,
        output: lines.join("\n"),
        metadata: receipt,
      };
    } catch (err) {
      return `error reading receipt: ${err}`;
    }
  },
});

export const c1forge_safety_scan = tool({
  description:
    "Run C1 Forge safety pipeline checks on the current worktree. Checks for forbidden " +
    "file edits, forbidden claims, secrets, and patch size limits. Returns safety hits and " +
    "verdict without modifying anything.",
  args: {},
  execute: async (args, context) => {
    const receiptPath = resolve(context.directory, ".selo", "runs");

    // List recent receipts to show safety status
    const { readdirSync, statSync } = await import("node:fs");
    if (!existsSync(receiptPath)) {
      return "No runs directory found. C1 Forge may not be initialized in this project.";
    }

    const entries = readdirSync(receiptPath)
      .filter((e: string) => e.startsWith("run-"))
      .sort()
      .reverse()
      .slice(0, 5);

    if (entries.length === 0) {
      return "No completed runs found. Submit a task first with c1forge_submit_task.";
    }

    const lines: string[] = ["Recent safety scan results:\n"];
    for (const entry of entries) {
      const recPath = resolve(receiptPath, entry, "receipt.json");
      if (existsSync(recPath)) {
        try {
          const data = readFileSync(recPath, "utf8");
          const rec = JSON.parse(data);
          const passed = rec.scans_passed ? "✅" : "❌";
          lines.push(`${passed} ${entry}: ${rec.final_verdict || rec.verdict} (${rec.duration_sec}s)`);
          if (rec.safety_hits && rec.safety_hits.length > 0) {
            lines.push(`   Hits: ${rec.safety_hits.join("; ")}`);
          }
        } catch {
          lines.push(`⚠️  ${entry}: could not read receipt`);
        }
      }
    }

    return {
      title: "C1 Forge safety scan results",
      output: lines.join("\n"),
      metadata: { recent_runs: entries },
    };
  },
});

export const c1forge_daemon_status = tool({
  description:
    "Check if the C1 Forge daemon is running and view queue statistics. " +
    "Shows pending, running, done, and failed task counts.",
  args: {},
  execute: async (args, context) => {
    const bin = resolveBinary();
    const proc = await execPromise(bin, ["status", "--json"]);

    if (proc.exitCode !== 0) {
      // Fallback: check queue directories manually
      const queueDir = resolve(context.directory, ".selo", "queue");
      if (!existsSync(queueDir)) {
        return "C1 Forge not initialized in this project. Run `selo init` first.";
      }

      const countDir = (dir: string): number => {
        const p = resolve(queueDir, dir);
        if (!existsSync(p)) return 0;
        return readdirSync(p).filter((f: string) => f.endsWith(".md")).length;
      };

      const pending = countDir("pending");
      const running = countDir("running");
      const done = countDir("done");
      const failed = countDir("failed");

      return {
        title: "C1 Forge queue status",
        output: `Pending: ${pending} | Running: ${running} | Done: ${done} | Failed: ${failed}`,
        metadata: { pending, running, done, failed },
      };
    }

    try {
      const status = JSON.parse(proc.stdout);
      return {
        title: "C1 Forge daemon status",
        output: [
          `Base dir: ${status.base_dir}`,
          `Daemon: ${status.lock_exists ? "running" : "not running"}`,
          `Pending: ${status.pending_count} tasks`,
          `Running: ${status.running_count} tasks`,
          `Done: ${status.done_count} tasks`,
          `Failed: ${status.failed_count} tasks`,
          status.recent_tasks?.length ? `Recent: ${status.recent_tasks.join(", ")}` : "",
        ].filter(Boolean).join("\n"),
        metadata: status,
      };
    } catch {
      return `error parsing status output: ${proc.stdout}`;
    }
  },
});
