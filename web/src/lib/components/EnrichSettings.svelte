<script lang="ts">
  import { api } from '../api';
  import { errorMessage } from '../errors';
  import { t } from '../i18n';
  import type { EnrichmentSettings } from '../api/types';

  // Turns AniList/MangaUpdates lookups on (RF-57). Off by default: the
  // series titles leave the local network.
  let enrichment = $state<EnrichmentSettings | null>(null);
  let notice = $state('');
  let error = $state('');

  $effect(() => {
    api.getSettings().then(
      (v) => (enrichment = v.enrichment),
      (err: unknown) => (error = errorMessage(err)),
    );
  });

  async function toggle(enabled: boolean) {
    notice = error = '';
    try {
      enrichment = (await api.saveSettings({ enrichment: { enabled } })).enrichment;
      notice = t('settings.enrich_saved');
    } catch (err) {
      error = errorMessage(err);
    }
  }
</script>

<fieldset>
  <legend>{t('settings.enrich')}</legend>
  <p class="help">{t('settings.enrich_help')}</p>
  {#if enrichment}
    <label>
      <input
        type="checkbox"
        checked={enrichment.enabled}
        disabled={enrichment.locked}
        onchange={(e) => toggle(e.currentTarget.checked)}
      />
      {t('settings.enrich_enable')}
    </label>
    {#if enrichment.locked}<p class="help">{t('settings.enrich_locked')}</p>{/if}
  {/if}
  {#if notice}<p role="status">{notice}</p>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
</fieldset>

<style>
  fieldset {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-top: 16px;
  }
  label {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .help {
    color: var(--text-muted);
    font-size: 0.85rem;
  }
  .error {
    color: var(--danger);
  }
</style>
