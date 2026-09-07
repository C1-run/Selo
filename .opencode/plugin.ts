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

export const id = "selo";
export default { id, server };
