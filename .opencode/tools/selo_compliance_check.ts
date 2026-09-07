import { tool } from "@opencode-ai/plugin";
import { z } from "zod";
import { dirname, resolve } from "node:path";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { execFile } from "node:child_process";

// Repo root: this file lives at <repo>/.opencode/tools/
const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");

// selo may not be on PATH; fall back to the repo-root binary.
function resolveBinary(): string {
  const repoBin = resolve(REPO_ROOT, "selo");
  if (existsSync(repoBin)) {
    return repoBin;
  }
  return "selo"; // rely on PATH
}

// Runs the check; a "stop" verdict exits 2 by design — that is not a tool failure.
function runCheck(bin: string, stepsFile: string): Promise<{ exitCode: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    execFile(
      bin,
      ["check", "--steps-file", stepsFile, "--format", "json"],
      { encoding: "utf8", maxBuffer: 10 * 1024 * 1024 },
      (err, stdout, stderr) => {
        const code = err ? (typeof (err as { code?: unknown }).code === "number" ? ((err as { code: number }).code) : 1) : 0;
        resolve({ exitCode: code, stdout: stdout ?? "", stderr: stderr ?? "" });
      },
    );
  });
}

export const selo_compliance_check = tool({
  description:
    "Check workspace changes for compliance against Selo's GateChain rules. " +
    "Runs `selo check --steps-file <path> --format json` on a steps.json file " +
    "containing an array of ChainStep objects and returns a structured pass/review/stop " +
    "verdict with blocking tools and warnings. Use this before shipping or merging when " +
    "Selo gating is in play.",
  args: {
    stepsFile: z
      .string()
      .describe(
        "Path to the steps.json file containing an array of ChainStep objects. " +
          "Relative paths are resolved against the project directory. " +
          "If omitted, defaults to samples/steps-pass.json.",
      )
      .optional(),
  },
  execute: async (args, context) => {
    const stepsFile = args.stepsFile
      ? resolve(context.directory, args.stepsFile)
      : resolve(context.directory, "samples", "steps-pass.json");

    if (!existsSync(stepsFile)) {
      return `error: steps file not found: ${stepsFile}`;
    }

    const proc = await runCheck(resolveBinary(), stepsFile);

    const stdout = proc.stdout.trim();
    if (proc.exitCode !== 0 && !stdout) {
      return `error: selo check failed (exit ${proc.exitCode}): ${proc.stderr.trim()}`;
    }

    let verdict: Record<string, unknown>;
    try {
      verdict = JSON.parse(stdout);
    } catch {
      return `error: could not parse selo output as JSON:\n${stdout}`;
    }

    const action = String(verdict.action ?? "unknown");
    const blockingTools: string[] = Array.isArray(verdict.blocking_tools)
      ? (verdict.blocking_tools as string[])
      : [];
    const warnings: string[] = Array.isArray(verdict.warnings)
      ? (verdict.warnings as string[])
      : [];

    const lines: string[] = [];
    switch (action) {
      case "pass":
        lines.push("✅ PASS — compliance check passed. All gates clear.");
        break;
      case "review":
        lines.push("🟡 REVIEW — compliance check requires human review before proceeding.");
        break;
      case "stop":
        lines.push("🔴 STOP — compliance check BLOCKED. Deployment cannot proceed.");
        break;
      default:
        lines.push(`⚠️ Unknown action: ${action}`);
    }
    if (verdict.action_reason) {
      lines.push(`Reason: ${String(verdict.action_reason)}`);
    }
    if (blockingTools.length > 0) {
      lines.push(`Blocking tools: ${blockingTools.join(", ")}`);
    }
    if (warnings.length > 0) {
      lines.push(`Warnings: ${warnings.join(", ")}`);
    }

    return {
      title: "Selo compliance check",
      output: lines.join("\n"),
      metadata: {
        action,
        final_status: verdict.final_status,
        stop_required: verdict.stop_required,
        review_required: verdict.review_required,
        blocking_tools: blockingTools,
        warnings,
      },
    };
  },
});
