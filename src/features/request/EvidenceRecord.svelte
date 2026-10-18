<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Pill from '../../ui/Pill.svelte';
  import Modal from '../../ui/Modal.svelte';
  import type { Job } from '../../lib/types';
  import { firstName } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';

  // The completion record: photos open a viewer, documents open privately, the note is a quote.
  let { job, workspaceId, title, viewer }: { job: Job; workspaceId: string | null; title: string; viewer: 'customer' | 'professional' } = $props();
  const pro = $derived(firstName(job.professional));
  const files = $derived(job.evidence.filter((item) => !!item.assetId));
  const photos = $derived(files.filter((item) => item.kind === 'photo' || item.contentType?.startsWith('image/')));
  const documents = $derived(files.filter((item) => !photos.includes(item)));
  const notes = $derived(job.evidence.filter((item) => item.kind === 'note'));
  const src = (assetId?: string) => `/api/evidence/${encodeURIComponent(assetId ?? '')}?workspaceId=${encodeURIComponent(workspaceId ?? '')}`;
  const fact = $derived(photos.length === 1 ? (notes.length ? t('1 photo and a note') : t('1 photo')) : notes.length ? t('{count} photos and a note', { count: photos.length }) : t('{count} photos', { count: photos.length }));

  let open = $state(false);
  let index = $state(0);
  function show(i: number) { index = i; open = true; }
</script>

<section class="record" aria-labelledby="record-title">
  <p class="meta">{t('Scope {version}',{version:job.scopeVersion})} · {job.paymentRefunded?t('Refunded'):job.paid?t('Paid'):t('Payment not captured')}</p>
  {#if job.completionApprovedAt}<p class="meta">{t('Completion approved')}: {job.completionApprovedAt}</p>{/if}
  <div class="record-head"><h2 id="record-title" class="section-title">{title}</h2><Pill variant="fact" label={fact}/></div>
  {#if photos.length}
    <div class="photos">
      {#each photos as photo, i (photo.assetId)}
        <button type="button" class="photo" aria-label={t('Open photo: {caption}', { caption: t('Photo {n}', { n: i + 1 }) })} onclick={() => show(i)}>
          <img src={src(photo.assetId)} alt="" loading="lazy"/>
          <span>{t('Photo {n}', { n: i + 1 })} · {t('Scope {version}',{version:photo.scopeVersion||1})}{#if (photo.scopeVersion && photo.scopeVersion!==job.scopeVersion)||(photo.completionRound||1)!==(job.completionRound||1)} · {t('Earlier completion attempt')}{/if}</span>
        </button>
      {/each}
    </div>
  {/if}
  {#if documents.length}
    <ul class="docs">
      {#each documents as doc, i (doc.assetId)}
        <li><a href={src(doc.assetId)} target="_blank" rel="noreferrer"><Icon name="file"/>{t('Document {n}', { n: i + 1 })}<Icon name="external" size={14}/></a></li>
      {/each}
    </ul>
  {/if}
  <div class="card note">
    <div class="note-head"><b>{viewer === 'customer' ? t('{name}’s note', { name: pro }) : t('Your work note')}</b>{#if !notes.length}<Pill variant="warn" label={t('Missing')}/>{/if}</div>
    {#if notes.length}{#each notes as note}<p class="meta">{t('Scope {version}',{version:note.scopeVersion||1})} · {t('Completion attempt {round}',{round:note.completionRound||1})}</p><p class="quote">{note.description}</p>{/each}
      <p class="meta"><Icon name="lock" size={13}/>{t('Private to you and {name}', { name: viewer === 'customer' ? pro : firstName(job.customer) })}</p>
    {:else}<p class="secondary-text">{viewer === 'customer' ? t('{name} hasn’t added a note yet.', { name: pro }) : t('Add a note that describes the work.')}</p>{/if}
  </div>
</section>

<Modal bind:open title={t('Photo {n}', { n: index + 1 })} wide>
  {#if photos[index]}
    <div class="viewer">
      <img src={src(photos[index].assetId)} alt={t('Photo {n}', { n: index + 1 })}/>
      <p class="meta"><Icon name="lock" size={13}/>{t('Sent by {name}', { name: pro })} · {t('Private to you and {name}', { name: viewer === 'customer' ? pro : firstName(job.customer) })}</p>
      {#if photos.length > 1}
        <div class="viewer-nav">
          <button type="button" class="icon-btn" aria-label={t('Previous photo')} disabled={index === 0} onclick={() => { index--; }}><Icon name="chevron-left"/></button>
          <span>{t('{n} of {total}', { n: index + 1, total: photos.length })}</span>
          <button type="button" class="icon-btn" aria-label={t('Next photo')} disabled={index === photos.length - 1} onclick={() => { index++; }}><Icon name="chevron-right"/></button>
        </div>
      {/if}
    </div>
  {/if}
</Modal>

<style>
  .record { display: grid; gap: 14px; }
  .record-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  .photos { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
  .photo { display: grid; gap: 6px; padding: 0; border: 0; background: transparent; text-align: left; color: var(--sk-ink); }
  .photo img { width: 100%; height: 110px; object-fit: cover; border-radius: var(--sk-r-card); background: var(--sk-surface-2); }
  .photo span { font-size: 12px; }
  .docs { display: grid; gap: 6px; }
  .docs a { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; font-weight: 600; }
  .note { display: grid; gap: 8px; padding: 16px; }
  .note-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .meta { display: flex; align-items: center; gap: 6px; }
  .viewer { display: grid; gap: 12px; }
  .viewer img { width: 100%; max-height: 500px; object-fit: contain; border-radius: var(--sk-r-card); background: var(--sk-surface-2); }
  .viewer-nav { display: flex; align-items: center; justify-content: center; gap: 12px; }
  @media (max-width: 639px) { .photos { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
