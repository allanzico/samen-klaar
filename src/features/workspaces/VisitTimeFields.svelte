<script lang="ts">
  import { defaultVisitEnd, localDateTime, validateVisit } from '../../lib/visit-time';
  import { t } from '../../lib/i18n.svelte';
  let {
    start = $bindable(''),
    end = $bindable(''),
    duration = 60,
    prefix = 'visit',
    error = $bindable(''),
    busy = false,
    leadMinutes = 15
  }: {
    start?: string;
    end?: string;
    duration?: number;
    prefix?: string;
    error?: string;
    busy?: boolean;
    leadMinutes?: number;
  } = $props();
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  let earliest = $derived(localDateTime(new Date(Date.now() + leadMinutes * 60000)));
  let endAdjusted = $state(false);
  let fieldError = $derived(error || (start && end ? validateVisit(start, end, duration) : ''));
  function setStart() {
    error = '';
    if (!endAdjusted || !end || new Date(end) <= new Date(start)) end = defaultVisitEnd(start, duration);
  }
</script>
<div class="visit-time-fields">
  <p id="{prefix}-help" class="quiet-note">{t('Times in')} {timezone}. {t('We allow {duration} minutes for this service; adjust the end if needed.', { duration })}</p>
  <label for="{prefix}-start">{t('Visit start')}<input id="{prefix}-start" type="datetime-local" bind:value={start} onchange={setStart} min={earliest} required disabled={busy} aria-describedby="{prefix}-help {prefix}-error" aria-invalid={fieldError ? 'true' : undefined}/></label>
  <label for="{prefix}-end">{t('Visit end')}<input id="{prefix}-end" type="datetime-local" bind:value={end} onchange={() => { endAdjusted = true; error = ''; }} min={defaultVisitEnd(start, duration) || earliest} required disabled={busy} aria-describedby="{prefix}-help {prefix}-error" aria-invalid={fieldError ? 'true' : undefined}/></label>
  <p id="{prefix}-error" class="field-error" role="alert">{fieldError}</p>
</div>
