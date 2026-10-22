<script lang="ts">
  import { api } from '../api';
  import { errorMessage } from '../errors';
  import { t } from '../i18n';
  import type { DesktopSettings } from '../api/types';

  // Desktop app only (Phase 6): this computer's library or a server's.
  // Applying "connected" opens the server in this same window.
  let {
    navigate = (url: string) => window.location.assign(url),
  }: { navigate?: (url: string) => void } = $props();

  let mode = $state<DesktopSettings['mode']>('standalone');
  let url = $state('');
  let notice = $state('');
  let error = $state('');

  $effect(() => {
    api.getSettings().then(
      (v) => {
        if (v.desktop) ({ mode, server_url: url } = v.desktop);
      },
      (err: unknown) => (error = errorMessage(err)),
    );
  });

  async function apply(e: SubmitEvent) {
    e.preventDefault();
    notice = error = '';
    try {
      const saved = (await api.saveSettings({ desktop: { mode, server_url: url } })).desktop;
      notice = t('settings.desktop.saved');
      if (saved?.mode === 'connected') navigate(saved.server_url + '/');
    } catch (err) {
      error = errorMessage(err);
    }
  }
</script>

<form onsubmit={apply}>
  <fieldset>
    <legend>{t('settings.desktop')}</legend>
    <p class="help">{t('settings.desktop_help')}</p>
    {#each ['standalone', 'connected'] as const as m (m)}
      <label class="radio">
        <input type="radio" name="desktop-mode" value={m} bind:group={mode} />
        {t(`settings.desktop.${m}`)}
      </label>
    {/each}
    {#if mode === 'connected'}
      <label>
        <span>{t('settings.desktop.url')}</span>
        <input type="url" placeholder="http://truenas.local:8080" bind:value={url} required />
      </label>
    {/if}
    <button type="submit">{t('settings.desktop.apply')}</button>
    {#if notice}<p role="status">{notice}</p>{/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  </fieldset>
</form>

<style>
  fieldset {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-top: 16px;
    display: grid;
    gap: 8px;
  }
  label {
    display: grid;
    gap: 3px;
    font-size: 0.9rem;
  }
  label.radio {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  input[type='url'] {
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }
  button {
    justify-self: start;
    padding: 8px 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    cursor: pointer;
  }
  .help {
    color: var(--text-muted);
    font-size: 0.85rem;
    margin: 0;
  }
  .error {
    color: var(--danger);
  }
</style>
