<script lang="ts">
  // Two houses: a tall left house and a shorter yellow one with a door.
  let { onNavy = false, size = 30, wordmark = true, name = 'SamenKlaar', large = false }: { onNavy?: boolean; size?: number; wordmark?: boolean; name?: string; large?: boolean } = $props();
</script>

<span class="logo" class:on-navy={onNavy} class:large>
  <svg viewBox="0 0 32 32" width={size} height={size} aria-hidden="true">
    <path d="M3 31V10.5L9.5 4 16 10.5V31z" fill={onNavy ? 'var(--sk-mark-on-brand)' : 'var(--sk-mark-a)'}/>
    <path d="M16 31V15.5L22.5 9 29 15.5V31z" fill="var(--sk-signal)"/>
    <path d="M20.5 31v-6h4v6z" fill="var(--sk-mark-door)"/>
  </svg>
  {#if wordmark}<span class="wordmark">{name}</span>{/if}
</span>

<style>
  .logo { display: inline-flex; align-items: center; gap: 10px; color: var(--sk-ink); }
  .logo.on-navy { color: var(--sk-on-brand); }
  .wordmark { font: 800 19px/1 var(--sk-fd); font-stretch: 90%; letter-spacing: -0.02em; white-space: nowrap; }
  .large .wordmark { font-size: 20px; }
</style>
