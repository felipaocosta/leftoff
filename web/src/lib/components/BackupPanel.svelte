<script lang="ts">
  import Spinner from './Spinner.svelte';
  import { api } from '../api';
  import { errorMessage } from '../errors';
  import { exportLibrary } from '../exportLibrary';
  import { t } from '../i18n';
  import { toasts } from '../stores/toast.svelte';

  // Export and import of the whole library (RF-43). Results also show as a
  // floating message, visible wherever the page is scrolled.
  let { mode = 'server' }: { mode?: string } = $props();
  let busy = $state(false);
  let exporting = $state(false);
  let message = $state('');
  let error = $state('');
  let fileInput: HTMLInputElement | undefined = $state();

  async function runExport() {
    exporting = true;
    try {
      const r = await exportLibrary(mode);
      if (r.kind === 'canceled') toasts.show(t('settings.export_canceled'));
      else if (r.kind === 'saved') toasts.show(t('settings.export_saved', { file: r.file }));
      else toasts.show(t('settings.export_downloaded', { file: r.file }));
    } catch (err) {
      toasts.show(t('settings.export_failed', { message: errorMessage(err) }), 'error');
    } finally {
      exporting = false;
    }
  }

  async function importFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow importing the same file again
    if (!file) return;
    busy = true;
    error = message = '';
    try {
      const r = await api.importLibrary(await file.text());
      message = t('settings.import_done', {
        added: r.added,
        merged: r.merged,
        read: r.chapters_read,
      });
      toasts.show(message);
    } catch (err) {
      error = errorMessage(err);
      toasts.show(error, 'error');
    } finally {
      busy = false;
    }
  }
</script>

<fieldset>
  <legend>{t('settings.backup')}</legend>
  <p class="help">{t('settings.backup_help')}</p>
  <div class="actions">
    <button type="button" onclick={runExport} disabled={exporting}>
      {#if exporting}<Spinner size={14} />{/if}
      {exporting ? t('settings.exporting') : t('settings.export')}
    </button>
    <button type="button" onclick={() => fileInput?.click()} disabled={busy}>
      {#if busy}<Spinner size={14} />{/if}
      {busy ? t('settings.importing') : t('settings.import')}
    </button>
    <input
      bind:this={fileInput}
      type="file"
      accept=".json,application/json"
      hidden
      onchange={importFile}
      aria-label={t('settings.import')}
    />
  </div>
  {#if message}<p role="status">{message}</p>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
</fieldset>

<style>
  fieldset {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-top: 16px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    cursor: pointer;
  }
  .help {
    color: var(--text-muted);
    font-size: 0.85rem;
  }
  .error {
    color: var(--danger);
  }
</style>
