<script lang="ts">
  import type { AppActions, AppState, Job } from '../../lib/types';
  import { t } from '../../lib/i18n.svelte';

  let { app, actions, job }: { app: AppState; actions: AppActions; job: Job } = $props();
  let rating = $state(5);
  let comment = $state('');
  let response = $state('');
  const ratingOptions = [{ value: 1, label: 'Very poor' }, { value: 2, label: 'Poor' }, { value: 3, label: 'Okay' }, { value: 4, label: 'Good' }, { value: 5, label: 'Excellent' }];

  async function submitReview(event: SubmitEvent) {
    event.preventDefault();
    if (await actions.act('submit-review', comment.trim(), undefined, undefined, undefined, rating)) comment = '';
  }

  async function submitResponse(event: SubmitEvent) {
    event.preventDefault();
    if (await actions.act('respond-to-review', response.trim())) response = '';
  }
</script>

{#if job.review}
  <section class="job-review" aria-label={t('Verified completed-job review')}>
    <p class="eyebrow">{t('VERIFIED COMPLETED JOB')}</p>
    <h3>{t('Customer review')}</h3>
    <p class="review-stars" aria-label={t('{rating} out of 5 stars', { rating: job.review.rating })}>{'★'.repeat(job.review.rating)}{'☆'.repeat(5 - job.review.rating)} <span>{job.review.rating}/5</span></p>
    {#if job.review.comment}<blockquote>{job.review.comment}</blockquote>{:else}<p class="quiet-note">{t('The customer left a rating without a written comment.')}</p>{/if}
    {#if job.review.professionalResponse}
      <div class="professional-response"><strong>{t('Professional response')}</strong><p>{job.review.professionalResponse}</p></div>
    {:else if app.role === 'professional'}
      <form class="stack" onsubmit={submitResponse}>
        <label for="review-response">{t('Your response')}<textarea id="review-response" bind:value={response} required minlength="3" maxlength="1000" rows="3" placeholder={t('Respond to the customer’s feedback.')}></textarea></label>
        <button disabled={app.pending || response.trim().length < 3}>{t('Post response')}</button>
      </form>
    {/if}
  </section>
{:else if app.role === 'customer' && job.paid && !job.paymentRefunded && job.completionApproved}
  <section class="job-review" aria-label={t('Review completed service')}>
    <p class="eyebrow">{t('COMPLETED SERVICE')}</p>
    <h3>{t('How did the service go?')}</h3>
    <p class="quiet-note">{t('Leave a verified review for this completed, paid job. You can submit one review.')}</p>
    <form class="stack" onsubmit={submitReview}>
      <fieldset class="rating">
        <legend>{t('Your rating')}</legend>
        <div class="stars">
          {#each ratingOptions as option (option.value)}
            <label class="star" class:on={option.value <= rating}>
              <input type="radio" name="job-review-rating" value={option.value} bind:group={rating} aria-label={`${option.value} — ${t(option.label)}`}/>
              <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3l2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1L3.2 9.5l6.1-.9L12 3Z"/></svg>
            </label>
          {/each}
        </div>
      </fieldset>
      <label for="job-review-comment">{t('Comment (optional)')}
        <textarea id="job-review-comment" bind:value={comment} maxlength="1000" rows="3" placeholder={t('Share what went well or what could improve.')}></textarea>
      </label>
      <button class="primary" disabled={app.pending}>{t('Post review')}</button>
    </form>
  </section>
{/if}
