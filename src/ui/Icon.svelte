<script lang="ts">
  import { ICONS, type IconName } from './icons';
  let { name, size, class: className = '' }: { name: IconName; size?: number; class?: string } = $props();
</script>

<svg class="icon {className}" viewBox="0 0 24 24" aria-hidden="true" style:width={size ? `${size}px` : undefined} style:height={size ? `${size}px` : undefined} stroke-width={name === 'more' ? 3 : name === 'check' ? 2.5 : undefined}><path d={ICONS[name]}/></svg>
