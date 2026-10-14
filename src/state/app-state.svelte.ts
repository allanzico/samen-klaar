import type { AppState } from '../lib/types';

export function createAppState(): AppState {
  const state = $state<AppState>({
    serverState: null,
    workspaces: [],
    workspaceCursor: null,
    workspaceId: null,
    session: null,
    providerConfig: null,
    professionalProfile: null,
    brandName: 'SamenKlaar',
    role: null,
    loading: false,
    pending: false,
    error: '',
    composeMode: 'message',
    draft: '',
    draftRestored: false,
    draftSaved: true,
    newConversation: false,
    clarificationAnswer: '',
    paypalStatus: 'not_started',
    evidenceFile: null
  });
  return state;
}
