import type { AppState } from '../lib/types';

export function createAppState(): AppState {
  const state = $state<AppState>({
    customerPage: 'home',
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
    messageTo: 'professional',
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
