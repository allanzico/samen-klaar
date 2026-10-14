// Tab-local drafts are scoped to the signed-in account, role and request.
const DRAFT_PREFIX = 'samenklaar:draft:';
// Preserve drafts created before the product rename; they are migrated on first read.
const LEGACY_DRAFT_PREFIX = 'relay:draft:';

export function draftKey(account: string | undefined, role: string | null, workspace: string | null, field = 'message') {
  return account && role && workspace ? `${DRAFT_PREFIX}${account}:${role}:${workspace}:${field}` : '';
}
export function readDraft(key: string): string {
  try {
    if (!key) return '';
    const current = sessionStorage.getItem(key);
    if (current !== null) return current;
    if (!key.startsWith(DRAFT_PREFIX)) return '';
    const legacyKey = LEGACY_DRAFT_PREFIX + key.slice(DRAFT_PREFIX.length);
    const legacy = sessionStorage.getItem(legacyKey);
    if (legacy === null) return '';
    try {
      sessionStorage.setItem(key, legacy);
      sessionStorage.removeItem(legacyKey);
    } catch { /* Keep returning the saved legacy value if migration cannot write. */ }
    return legacy;
  } catch { return ''; }
}
export function saveDraft(key: string, value: string): boolean {
  try { if (!key) return false; if (value) sessionStorage.setItem(key, value); else sessionStorage.removeItem(key); return true; } catch { return false; }
}
export function clearAccountDrafts(account: string | undefined) {
  try {
    for (const key of Object.keys(sessionStorage)) {
      if (account && (key.startsWith(`${DRAFT_PREFIX}${account}:`) || key.startsWith(`${LEGACY_DRAFT_PREFIX}${account}:`))) sessionStorage.removeItem(key);
    }
  } catch { /* Browser storage may be unavailable. */ }
}

export function readFormDraft(key: string): Record<string, string> {
  try { const value: unknown = JSON.parse(readDraft(key) || '{}');
    const fields: Record<string, string> = {};
    if (value && typeof value === 'object') for (const [name, field] of Object.entries(value)) if (typeof field === 'string') fields[name] = field;
    return fields;
  } catch { return {}; }
}
