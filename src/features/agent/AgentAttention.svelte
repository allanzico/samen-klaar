<script lang="ts">
  import Banner from '../../ui/Banner.svelte';
  import { agentUi } from '../../lib/agent.svelte';
  import { questionText } from '../../lib/agent-copy';
  import { t } from '../../lib/i18n.svelte';

  // A short line under the app bar when an assistant stopped and needs its person, named by its bot name.
  // Opens that request.
  let { onOpen }: { onOpen: (workspaceId: string) => void } = $props();
  const first = $derived(agentUi.escalations[0]);
  const count = $derived(agentUi.escalations.length);
  const name = $derived(first?.params?.agent);
  const title = $derived(
    count > 1
      ? (name ? t('{name} needs you on {count} requests', { name, count }) : t('Your assistant needs you on {count} requests', { count }))
      : (name ? t('{name} needs you', { name }) : t('Your assistant needs you'))
  );
</script>

{#if first}
  <div class="attention">
    <Banner kind="info" {title}>
      <p>{questionText(first)} <button type="button" class="link" onclick={() => onOpen(first.workspaceId)}>{t('Open the request')}</button></p>
    </Banner>
  </div>
{/if}

<style>
  .attention { max-width: 1040px; margin: 12px auto 0; padding: 0 16px; width: 100%; }
  .link { display: inline-flex; align-items: center; min-height: 32px; padding: 0; border: 0; background: none; font: inherit; font-weight: 600; color: inherit; text-decoration: underline; cursor: pointer; }
  @media (max-width: 639px) { .link { min-height: 44px; } }
</style>
