<script lang="ts">
  import Banner from '../../ui/Banner.svelte';
  import { agentUi } from '../../lib/agent.svelte';
  import { questionText } from '../../lib/agent-copy';
  import { t } from '../../lib/i18n.svelte';

  // A short line under the app bar when an agent stopped and needs its person. Opens that request.
  let { onOpen }: { onOpen: (workspaceId: string) => void } = $props();
  const first = $derived(agentUi.escalations[0]);
</script>

{#if first}
  <div class="attention">
    <Banner kind="warn" title={agentUi.escalations.length > 1 ? t('Your agent needs you on {count} requests', { count: agentUi.escalations.length }) : t('Your agent needs you')}>
      <p>{questionText(first)} <button type="button" class="link" onclick={() => onOpen(first.workspaceId)}>{t('Open the request')}</button></p>
    </Banner>
  </div>
{/if}

<style>
  .attention { max-width: 1040px; margin: 12px auto 0; padding: 0 16px; width: 100%; }
  .link { padding: 0; border: 0; background: none; font: inherit; font-weight: 600; color: inherit; text-decoration: underline; cursor: pointer; }
</style>
