<script lang="ts">
  import type { Job } from '../../lib/types';
  import { money } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';

  // "What you agreed": the current scope's lines and total.
  let { job, title = 'What you agreed' }: { job: Job; title?: string } = $props();
</script>

<section class="box agreed" aria-label={t(title)}>
  <h2 class="small-title">{t(title)}</h2>
  <dl class="ledger">
    {#each job.quote.lines as line (line.label)}<div><dt>{t(line.label)}</dt><dd>{money(line.amountMinor, job.quote.currency)}</dd></div>{/each}
    <div class="ledger-total"><dt>{t('Total')}</dt><dd>{money(job.quote.totalMinor, job.quote.currency)}</dd></div>
  </dl>
</section>

<style>
  .agreed { display: grid; gap: 8px; padding: 18px 22px; }
</style>
