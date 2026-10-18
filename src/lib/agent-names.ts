// An assistant's short bot name. Must match internal/workflow/agent_names.go: the name is a hash of the
// person's name, so the same person's assistant is called the same thing before and after a match.
const NAMES = ['Pip', 'Juno', 'Kit', 'Remy', 'Nova', 'Finn', 'Mika', 'Luca', 'Noor', 'Teo', 'Ivy', 'Sky', 'Rio', 'Ada', 'Zed', 'Bo'];

function fnv1a(text: string): number {
  let hash = 0x811c9dc5;
  for (const byte of new TextEncoder().encode(text)) {
    hash ^= byte;
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash >>> 0;
}

export function assistantName(seed: string): string {
  return NAMES[fnv1a(seed.trim().toLowerCase()) % NAMES.length];
}
