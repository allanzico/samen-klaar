<script lang="ts">
  import Icon from '../../ui/Icon.svelte';
  import Modal from '../../ui/Modal.svelte';
  import type { AppActions, AppState, WorkspaceState } from '../../lib/types';
  import { firstName } from '../../lib/status-ui';
  import { t } from '../../lib/i18n.svelte';

  // G10: private photos the customer attached to describe the problem. Only the customer and, once
  // chosen, the assigned professional can see them; they are not sent to the AI and are not completion proof.
  let { app, actions, workspace }: { app: AppState; actions: AppActions; workspace: WorkspaceState } = $props();
  const photos = $derived(workspace.requestPhotos ?? []);
  const customer = $derived(app.role === 'customer');
  const canAdd = $derived(customer && !workspace.cancelled && !workspace.job.paid && photos.length < 6);
  const pro = $derived(firstName(workspace.job.professional));
  let open = $state(false);
  let index = $state(0);
  let input = $state<HTMLInputElement>();
  const src = (assetId: string) => `/api/request-photos/${encodeURIComponent(assetId)}?workspaceId=${encodeURIComponent(workspace.workspaceId)}`;

  async function pick(event: Event) {
    const el = event.currentTarget as HTMLInputElement;
    const file = el.files?.[0];
    el.value = '';
    if (file) await actions.uploadRequestPhoto(file);
  }
</script>

{#if photos.length || canAdd}
  <section class="photos" aria-labelledby="photos-title">
    <div class="head">
      <h2 id="photos-title" class="small-title">{customer ? t('Photos of the problem') : t('Photos from the customer')}</h2>
      {#if canAdd}
        <button type="button" class="btn btn-light btn-sm" disabled={app.pending} onclick={() => input?.click()}><Icon name="camera"/>{t('Add photos')}</button>
        <input bind:this={input} class="visually-hidden" type="file" accept="image/jpeg,image/png,image/webp" tabindex="-1" onchange={pick}/>
      {/if}
    </div>
    {#if photos.length}
      <div class="grid">
        {#each photos as photo, i (photo.assetId)}
          <button type="button" class="photo" aria-label={t('Open photo {n}', { n: i + 1 })} onclick={() => { index = i; open = true; }}><img src={src(photo.assetId)} alt="" loading="lazy"/></button>
        {/each}
      </div>
      <p class="meta"><Icon name="lock" size={13}/>{customer ? (workspace.job.id ? t('Private to you and {name}.', { name: pro }) : t('Private to you. A professional sees them only after you choose them.')) : t('Private to you and the customer.')}</p>
    {:else}
      <p class="secondary-text">{t('A photo helps the professional bring the right parts. Add up to six.')}</p>
    {/if}
  </section>
{/if}

<Modal bind:open title={t('Photo {n}', { n: index + 1 })} wide>
  {#if photos[index]}
    <div class="viewer"><img src={src(photos[index].assetId)} alt={t('Photo {n}', { n: index + 1 })}/>
      {#if photos.length > 1}
        <div class="nav">
          <button type="button" class="icon-btn" aria-label={t('Previous photo')} disabled={index === 0} onclick={() => { index--; }}><Icon name="chevron-left"/></button>
          <span>{t('{n} of {total}', { n: index + 1, total: photos.length })}</span>
          <button type="button" class="icon-btn" aria-label={t('Next photo')} disabled={index === photos.length - 1} onclick={() => { index++; }}><Icon name="chevron-right"/></button>
        </div>
      {/if}
    </div>
  {/if}
</Modal>

<style>
  .photos { display: grid; gap: 10px; max-width: 820px; width: 100%; margin: 0 auto; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .grid { display: flex; flex-wrap: wrap; gap: 10px; }
  .photo { width: 96px; height: 96px; padding: 0; border: 0; border-radius: var(--sk-r-card); overflow: hidden; background: var(--sk-surface-2); }
  .photo img { width: 100%; height: 100%; object-fit: cover; }
  .meta { display: flex; align-items: center; gap: 6px; }
  .viewer { display: grid; gap: 10px; }
  .viewer img { width: 100%; max-height: 520px; object-fit: contain; border-radius: var(--sk-r-card); background: var(--sk-surface-2); }
  .nav { display: flex; align-items: center; justify-content: center; gap: 12px; }
</style>
