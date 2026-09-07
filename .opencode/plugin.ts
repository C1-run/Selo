import type { Plugin, Hooks } from "@opencode-ai/plugin";
import { selo_compliance_check } from "./tools/selo_compliance_check";
import {
  selo_submit_task,
  selo_task_status,
  selo_safety_scan,
  selo_daemon_status,
} from "./tools/selo_plugin_tools";

const server: Plugin = async (): Promise<Hooks> => {
  return {
    tool: {
      selo_compliance_check,
      selo_submit_task,
      selo_task_status,
      selo_safety_scan,
      selo_daemon_status,
    },
  };
};

export const id = "selo";
export default { id, server };
