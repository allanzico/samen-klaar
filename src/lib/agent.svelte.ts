import { api } from './api';
import type { AgentEscalation } from './types';

// Shared between the account menu (opens the settings), the app shell (attention banner) and the
// request page (the questions for the open request).
export const agentUi = $state({ settingsOpen: false, escalations: [] as AgentEscalation[] });

export async function refreshAgentEscalations() {
  try { agentUi.escalations = await api.agentEscalations(); }
  catch { /* the banner is a convenience; the request page still shows its own questions */ }
}
