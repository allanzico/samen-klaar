export type StepButton = { label: string; run?: () => unknown; href?: string; disabled?: boolean; reason?: string };
export type StepView = {
  kind: 'your-turn' | 'waiting' | 'scheduled' | 'done' | 'info';
  title: string; text?: string; amount?: string;
  banner?: { kind: 'error' | 'info'; title?: string; text: string };
  ledger?: [string, string][];
  steps?: { text: string; state: 'done' | 'current' | 'todo' }[];
  list?: { title: string; items: string[] };
  primary?: StepButton; secondary?: StepButton; quiet?: StepButton[]; danger?: StepButton; helper?: string;
};
