<script lang="ts">
  import BackupPanel from '../lib/components/BackupPanel.svelte';
  import DesktopMode from '../lib/components/DesktopMode.svelte';
  import EnrichSettings from '../lib/components/EnrichSettings.svelte';
  import NotificationSettings from '../lib/components/NotificationSettings.svelte';
  import { href } from '../lib/stores/router.svelte';
  import { api } from '../lib/api';
  import { i18n, locales, t, type Locale } from '../lib/i18n';
  import { prefs, type Theme } from '../lib/stores/prefs.svelte';

  const themes: Theme[] = ['system', 'light', 'dark'];
  const languages = Object.keys(locales) as Locale[];
  let token = $state(prefs.token);
  let saved = $state(false);
  let version = $state('');
  let appMode = $state(false);
  let desktop = $state(false);
  let mode = $state('server');
  let quitted = $state(false);

  $effect(() => {
    api.health().then(
      (h) => {
        version = h.version;
        appMode = h.mode === 'app' || h.mode === 'desktop';
        desktop = h.mode === 'desktop' || h.mode === 'android';
        mode = h.mode;
      },
      () => (version = ''),
    );
  });

  async function quit() {
    await api.quitApp();
    quitted = true;
  }

  function save(e: SubmitEvent) {
    e.preventDefault();
    prefs.setToken(token);
    saved = true;
  }
</script>

<section>
  <h1>{t('settings.title')}</h1>

  <form onsubmit={save}>
    <label for="token">{t('settings.token')}</label>
    <div class="row">
      <input
        id="token"
        type="password"
        autocomplete="off"
        bind:value={token}
        oninput={() => (saved = false)}
      />
      <button type="submit">{t('settings.save')}</button>
    </div>
    <p class="help">{t('settings.token_help')}</p>
    <p aria-live="polite">{saved ? t('settings.saved') : ''}</p>
  </form>

  <fieldset>
    <legend>{t('settings.theme')}</legend>
    {#each themes as th (th)}
      <label class="radio">
        <input
          type="radio"
          name="theme"
          value={th}
          checked={prefs.theme === th}
          onchange={() => prefs.setTheme(th)}
        />
        {t(`settings.theme.${th}`)}
      </label>
    {/each}
  </fieldset>

  <fieldset>
    <legend>{t('settings.language')}</legend>
    {#each languages as lang (lang)}
      <label class="radio" {lang}>
        <input
          type="radio"
          name="language"
          value={lang}
          checked={i18n.locale === lang}
          onchange={() => i18n.set(lang)}
        />
        {locales[lang].name}
      </label>
    {/each}
  </fieldset>

  {#if desktop}<DesktopMode />{/if}

  <NotificationSettings />

  <EnrichSettings />

  <BackupPanel {mode} />

  <fieldset>
    <legend>{t('settings.recipes')}</legend>
    <p class="help">{t('settings.recipes_help')}</p>
    <a href={href({ name: 'recipes' })}>{t('recipes.title')} →</a>
  </fieldset>

  {#if appMode}
    <fieldset>
      <legend>{t('settings.app')}</legend>
      <p class="help">{t('settings.app_help')}</p>
      {#if quitted}
        <p role="status">{t('settings.quit_done')}</p>
      {:else}
        <button type="button" class="danger" onclick={quit}>{t('settings.quit')}</button>
      {/if}
    </fieldset>
  {/if}

  {#if version}<p class="help">{t('settings.server', { version })}</p>{/if}
</section>

<style>
  section {
    max-width: 560px;
  }
  .row {
    display: flex;
    gap: 8px;
  }
  input[type='password'] {
    flex: 1;
    min-width: 0;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
  }
  button {
    padding: 8px 14px;
    border: 0;
    border-radius: 8px;
    background: var(--accent);
    color: var(--accent-text);
    cursor: pointer;
  }
  fieldset {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-top: 16px;
  }
  .radio {
    display: block;
    padding: 4px 0;
  }
  .danger {
    background: var(--danger);
    color: #fff;
  }
  .help {
    color: var(--text-muted);
    font-size: 0.85rem;
  }
</style>
