<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import type { AppActions, AppState, Job } from '../../lib/types';
  import { firstName } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';

  // Verified completed-job review (D12, H08, F20): one customer rating, one professional response.
  let { app, actions, job }: { app: AppState; actions: AppActions; job: Job } = $props();
  let rating = $state(0);
  let comment = $state('');
  let response = $state('');
  const labels = ['Very poor', 'Poor', 'Okay', 'Good', 'Excellent'];

  async function submitReview(event: SubmitEvent) {
    event.preventDefault();
    if (rating && await actions.act('submit-review', comment.trim(), undefined, undefined, undefined, rating)) comment = '';
  }
  async function submitResponse(event: SubmitEvent) {
    event.preventDefault();
    if (await actions.act('respond-to-review', response.trim())) response = '';
  }
</script>

{#if job.review}
  <section class="box review" aria-labelledby="review-title">
    <div class="review-head"><h2 id="review-title" class="small-title">{app.role === 'customer' ? t('Your review') : t('{name}’s review', { name: firstName(job.customer) })}</h2><Pill variant="done" label={t('Verified completed job')}/></div>
    <p class="stars" role="img" aria-label={t('{rating} out of 5 stars', { rating: job.review.rating })}>
      {#each [1, 2, 3, 4, 5] as value}<span class:on={value <= job.review.rating}><Icon name="star"/></span>{/each}
    </p>
    {#if job.review.comment}<p class="quote">{job.review.comment}</p>{/if}
    {#if job.review.professionalResponse}
      <div class="response"><b>{app.role === 'professional' ? t('Your response') : t('{name}’s response', { name: firstName(job.professional) })}</b><p>{job.review.professionalResponse}</p></div>
    {:else if app.role === 'professional'}
      <form class="form" onsubmit={submitResponse}>
        <div class="field"><label for="review-response">{t('Your response')}</label><textarea id="review-response" class="textarea" rows="3" required minlength="3" maxlength="1000" bind:value={response}></textarea></div>
        <button class="btn btn-primary btn-sm" disabled={app.pending || response.trim().length < 3}>{t('Post response')}</button>
        {#if response.trim().length < 3}<p class="helper">{t('Write at least 3 characters to post.')}</p>{/if}
      </form>
    {/if}
  </section>
{:else if app.role === 'customer' && job.paid && !job.paymentRefunded && job.completionApproved}
  <section class="box review" aria-labelledby="review-title">
    <h2 id="review-title" class="small-title">{t('How did it go?')}</h2>
    <form class="form" onsubmit={submitReview}>
      <fieldset class="rating">
        <legend class="field-label">{t('Your rating')}</legend>
        <div class="stars-input">
          {#each labels as label, i}
            <label class="star" class:on={i + 1 <= rating}>
              <input type="radio" name="job-rating" value={i + 1} bind:group={rating}/>
              <Icon name="star"/><span class="visually-hidden">{i + 1} — {t(label)}</span>
            </label>
          {/each}
        </div>
      </fieldset>
      <div class="field"><label for="review-comment">{t('Comment (optional)')}</label><textarea id="review-comment" class="textarea" rows="3" maxlength="1000" bind:value={comment}></textarea></div>
      <button class="btn btn-primary btn-sm" disabled={app.pending || !rating}>{t('Post review')}</button>
      {#if !rating}<p class="helper">{t('Choose a rating to post your review.')}</p>{/if}
    </form>
  </section>
{/if}

<style>
  .review { display: grid; gap: 12px; padding: 20px 22px; }
  .review-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  .stars, .stars-input { display: flex; gap: 4px; }
  .stars span, .star { color: var(--sk-field); }
  .stars span.on, .star.on { color: var(--sk-signal); }
  .stars span.on :global(.icon), .star.on :global(.icon) { fill: var(--sk-signal); stroke: var(--sk-on-signal); }
  .star { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 50%; cursor: pointer; }
  .star :global(.icon) { width: 26px; height: 26px; }
  .star input { position: absolute; opacity: 0; pointer-events: none; }
  .star:has(input:focus-visible) { outline: 2px solid var(--sk-ink); outline-offset: 1px; }
  .form { display: grid; gap: 12px; justify-items: start; }
  .form .field { width: 100%; }
  .rating { display: grid; gap: 6px; }
  .response { display: grid; gap: 4px; padding: 12px 14px; border-radius: var(--sk-r-card); background: var(--sk-ground); }
</style>
