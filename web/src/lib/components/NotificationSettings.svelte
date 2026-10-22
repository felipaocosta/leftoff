<script lang="ts">
  import Spinner from './Spinner.svelte';
  import { api } from '../api';
  import { errorMessage } from '../errors';
  import { locales, t, type MessageKey } from '../i18n';
  import type { ChannelResult, SettingsView } from '../api/types';

  // New-chapter notifications (RF-26): the three channels, the filter and
  // the language, plus a test message. Fields set by environment variables
  // are shown but cannot be edited.
  type TextField = 'ntfy_url' | 'discord_webhook' | 'telegram_token' | 'telegram_chat_id';
  const fields: { name: TextField; locked: string; label: MessageKey; help?: MessageKey }[] = [
    {
      name: 'ntfy_url',
      locked: 'ntfy_url',
      label: 'settings.notify.ntfy',
      help: 'settings.notify.ntfy_help',
    },
    {
      name: 'discord_webhook',
      locked: 'discord_webhook',
      label: 'settings.notify.discord',
      help: 'settings.notify.discord_help',
    },
    { name: 'telegram_token', locked: 'telegram.token', label: 'settings.notify.telegram_token' },
    {
      name: 'telegram_chat_id',
      locked: 'telegram.chat_id',
      label: 'settings.notify.telegram_chat',
      help: 'settings.notify.telegram_help',
    },
  ];

  let form = $state({
    ntfy_url: '',
    discord_webhook: '',
    telegram_token: '',
    telegram_chat_id: '',
    only_reading: false,
    language: 'pt-BR' as 'pt-BR' | 'en',
  });
  let locked = $state<string[]>([]);
  let loaded = $state(false);
  let busy = $state<'' | 'save' | 'test'>('');
  let notice = $state('');
  let error = $state('');
  let results = $state<ChannelResult[]>([]);

  function apply(v: SettingsView) {
    const n = v.notifications;
    form = {
      ntfy_url: n.ntfy_url,
      discord_webhook: n.discord_webhook,
      telegram_token: n.telegram.token,
      telegram_chat_id: n.telegram.chat_id,
      only_reading: n.only_reading,
      language: n.language,
    };
    locked = n.locked ?? [];
  }

  $effect(() => {
    api.getSettings().then(
      (v) => {
        apply(v);
        loaded = true;
      },
      (err: unknown) => (error = errorMessage(err)),
    );
  });

  function save(): Promise<SettingsView> {
    return api.saveSettings({
      notifications: {
        ntfy_url: form.ntfy_url,
        discord_webhook: form.discord_webhook,
        telegram: { token: form.telegram_token, chat_id: form.telegram_chat_id },
        only_reading: form.only_reading,
        language: form.language,
      },
    });
  }

  async function run(kind: 'save' | 'test') {
    busy = kind;
    notice = error = '';
    results = [];
    try {
      apply(await save());
      if (kind === 'test') results = await api.testNotifications();
      else notice = t('settings.notify.saved');
    } catch (err) {
      error = errorMessage(err);
    } finally {
      busy = '';
    }
  }
</script>

<fieldset>
  <legend>{t('settings.notify')}</legend>
  <p class="help">{t('settings.notify_help')}</p>
  {#if loaded}
    {#each fields as f (f.name)}
      <label>
        <span>{t(f.label)}</span>
        <input
          type="text"
          autocomplete="off"
          spellcheck="false"
          bind:value={form[f.name]}
          disabled={locked.includes(f.locked)}
        />
        {#if locked.includes(f.locked)}<small>{t('settings.notify.locked')}</small>{/if}
        {#if f.help}<small class="help">{t(f.help)}</small>{/if}
      </label>
    {/each}
    <label class="check">
      <input type="checkbox" bind:checked={form.only_reading} />
      {t('settings.notify.only_reading')}
    </label>
    <label>
      <span>{t('settings.notify.language')}</span>
      <select bind:value={form.language}>
        <option value="pt-BR">{locales['pt-BR'].name}</option>
        <option value="en">{locales.en.name}</option>
      </select>
    </label>
    <div class="actions">
      <button type="button" onclick={() => run('save')} disabled={busy !== ''}>
        {#if busy === 'save'}<Spinner size={14} />{/if}
        {t('settings.notify.save')}
      </button>
      <button type="button" onclick={() => run('test')} disabled={busy !== ''}>
        {#if busy === 'test'}<Spinner size={14} />{/if}
        {busy === 'test' ? t('settings.notify.testing') : t('settings.notify.test')}
      </button>
    </div>
  {/if}
  {#if notice}<p role="status">{notice}</p>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
  {#if results.length > 0}
    <ul class="results" role="status">
      {#each results as r (r.channel)}
        <li class:fail={!r.ok}>
          {r.ok
            ? t('settings.notify.test_ok', { channel: r.channel })
            : t('settings.notify.test_fail', { channel: r.channel, error: r.error ?? '' })}
        </li>
      {/each}
    </ul>
  {/if}
</fieldset>

<style>
  fieldset {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-top: 16px;
    display: grid;
    gap: 10px;
  }
  label {
    display: grid;
    gap: 3px;
    font-size: 0.85rem;
  }
  label.check {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  input[type='text'],
  select {
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface);
    font-family: inherit;
  }
  input:disabled {
    background: var(--bg);
    color: var(--text-muted);
    cursor: not-allowed;
  }
  small,
  .help {
    color: var(--text-muted);
    font-size: 0.8rem;
    margin: 0;
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
  .results {
    margin: 0;
    padding-left: 18px;
    color: var(--success);
  }
  .results .fail,
  .error {
    color: var(--danger);
  }
</style>
