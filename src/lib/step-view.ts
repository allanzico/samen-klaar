import type { PillVariant } from './status-ui';

// "Your next step": one view model rendered in the web aside, the dock above the composer
// below 1280 px, and the phone turn card.
export type StepButton = { label: string; run?: () => unknown; href?: string; disabled?: boolean; reason?: string; busyLabel?: string };
export type StepView = {
  kind: 'your-turn' | 'waiting' | 'info';
  tag: { variant: PillVariant; label: string };
  title: string;
  text?: string;
  amount?: string;
  banner?: { kind: 'error' | 'info' | 'warn' | 'success'; title?: string; text: string };
  ledger?: { label: string; value: string; total?: boolean }[];
  primary?: StepButton;
  secondary?: StepButton;
  quiet?: StepButton[];
  danger?: StepButton;
  helper?: string;
  // "After this" rows: the current step first (6.10).
  after?: string[];
  list?: { title: string; items: string[] };
};
