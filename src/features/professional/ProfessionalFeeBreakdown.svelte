<script lang="ts">
  import { professionalPayment } from '../../lib/platform-fee';
  import { money } from '../../lib/format';
  import { t } from '../../lib/i18n.svelte';
  let { totalMinor, currency, basisPoints = 0 }: { totalMinor: number; currency: string; basisPoints?: number } = $props();
  const payment = $derived(professionalPayment(totalMinor, basisPoints));
</script>

{#if payment && basisPoints > 0}
  <section class="box payout" aria-label={t('Your payout')}>
    <h3 class="small-title">{t('Your payout')}</h3>
    <dl class="ledger">
      <div><dt>{t('Customer total')}</dt><dd>{money(payment.totalMinor, currency)}</dd></div>
      <div><dt>{t('SamenKlaar fee ({percent}%)', { percent: basisPoints / 100 })}</dt><dd>−{money(payment.feeMinor, currency)}</dd></div>
      <div class="ledger-total"><dt>{t('You receive before PayPal fees')}</dt><dd>{money(payment.proceedsMinor, currency)}</dd></div>
    </dl>
    <p class="helper">{t('The platform fee is deducted from your agreed total, including call-out. PayPal processing fees are separate.')}</p>
  </section>
{/if}

<style>.payout { display: grid; gap: 12px; padding: 20px 22px; }</style>
