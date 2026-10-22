<script lang="ts">
  import { t } from '../i18n';
  import { prefs, type LibraryView } from '../stores/prefs.svelte';

  // Grid or compact list for the library (RF-31), kept in this browser.
  const views: LibraryView[] = ['grid', 'list'];
</script>

<div class="views" role="group" aria-label={t('library.view')}>
  {#each views as v (v)}
    <button type="button" aria-pressed={prefs.view === v} onclick={() => prefs.setView(v)}>
      {t(`library.view.${v}`)}
    </button>
  {/each}
</div>

<style>
  .views {
    display: flex;
  }
  button {
    padding: 4px 10px;
    font-size: 0.8rem;
    border: 1px solid var(--border);
    background: var(--surface);
    cursor: pointer;
  }
  button:first-child {
    border-radius: 8px 0 0 8px;
  }
  button:last-child {
    border-radius: 0 8px 8px 0;
    border-left: 0;
  }
  button[aria-pressed='true'] {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-text);
  }
</style>
