<script lang="ts">
  import Icon from './Icon.svelte';
  import type { IconName } from './icons';
  import type { PillVariant } from '../lib/status-ui';

  let { variant, label, icon, light = false, row = false, small = false }: { variant: PillVariant; label: string; icon?: IconName; light?: boolean; row?: boolean; small?: boolean } = $props();
  // Dots carry state for your-turn, waiting, pending and dense booked rows; icons for the rest.
  const dot = $derived(!icon && (variant === 'your-turn' || variant === 'waiting' || variant === 'pending' || (variant === 'booked' && light)));
  const autoIcon = $derived<IconName | undefined>(icon ?? (variant === 'done' ? 'check' : variant === 'warn' || variant === 'error' ? 'alert' : variant === 'best' ? 'star' : undefined));
</script>

<span class="pill pill--{variant}" class:pill--light={light} class:pill--row={row} class:pill--small={small}>
  {#if dot}<span class="pill-dot" aria-hidden="true"></span>{:else if autoIcon}<Icon name={autoIcon}/>{/if}{label}
</span>
