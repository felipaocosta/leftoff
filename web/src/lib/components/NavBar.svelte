<script lang="ts">
  import { t, type MessageKey } from '../i18n';
  import { news } from '../stores/news.svelte';
  import { href, type Route } from '../stores/router.svelte';

  // Main navigation: a top bar on wide screens and, on phones, a bottom
  // tab bar with icons (thumb reach, like native apps). Same links, so
  // keyboard and screen readers get one list either way.
  let { current }: { current: Route['name'] } = $props();

  // short: the phone tab bar label, when the full one does not fit.
  type Item = {
    name: 'library' | 'news' | 'add' | 'settings' | 'about';
    label: MessageKey;
    short?: MessageKey;
    icon: string;
  };
  // 24×24 stroke icons.
  const items: Item[] = [
    {
      name: 'library',
      label: 'nav.library',
      icon: 'M4 5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1zm10 0a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4a1 1 0 0 1-1-1z',
    },
    {
      name: 'news',
      label: 'nav.news',
      icon: 'M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0',
    },
    { name: 'add', label: 'nav.add_short', icon: 'M12 5v14M5 12h14' },
    {
      name: 'settings',
      label: 'nav.settings',
      short: 'nav.settings_short',
      icon: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z',
    },
    {
      name: 'about',
      label: 'nav.about',
      icon: 'M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 16v-4M12 8h.01',
    },
  ];
</script>

<header>
  <nav aria-label={t('nav.main')}>
    <a class="brand" href={href({ name: 'library' })}>{t('app.title')}</a>
    {#each items as item (item.name)}
      <a
        class="item item-{item.name}"
        href={href({ name: item.name } as Route)}
        aria-current={current === item.name ? 'page' : undefined}
      >
        <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path d={item.icon} /></svg>
        <span class="label" class:long={item.short}>{t(item.label)}</span>
        {#if item.short}<span class="label short">{t(item.short)}</span>{/if}
        {#if item.name === 'news' && news.count > 0}
          <span class="badge" aria-label={t('nav.news_count', { n: news.count })}>{news.count}</span
          >
        {/if}
      </a>
    {/each}
  </nav>
</header>

<style>
  header {
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    z-index: 10;
  }
  nav {
    max-width: 1200px;
    margin: 0 auto;
    padding: 10px 16px;
    display: flex;
    gap: 16px;
    align-items: center;
  }
  a {
    text-decoration: none;
    color: var(--text-muted);
    white-space: nowrap;
    position: relative;
  }
  a[aria-current='page'] {
    color: var(--text);
    font-weight: 600;
  }
  .brand {
    margin-right: auto;
    font-weight: 700;
    color: var(--text);
  }
  .icon,
  .item-library,
  .short {
    display: none;
  }
  .badge {
    margin-left: 4px;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--accent);
    color: var(--accent-text);
    font-size: 0.75rem;
    font-weight: 700;
  }

  /* Phones: a bottom tab bar above the system gesture area. */
  @media (max-width: 640px) {
    header {
      position: fixed;
      top: auto;
      bottom: 0;
      left: 0;
      right: 0;
      border-bottom: 0;
      border-top: 1px solid var(--border);
      padding-bottom: env(safe-area-inset-bottom);
    }
    nav {
      padding: 4px 0;
      gap: 0;
      justify-content: space-around;
    }
    .brand {
      display: none;
    }
    .item,
    .item-library {
      display: flex;
      flex: 1;
      flex-direction: column;
      align-items: center;
      gap: 2px;
      padding: 6px 2px;
      font-size: 0.7rem;
      min-width: 0;
    }
    a[aria-current='page'] {
      color: var(--accent);
    }
    .icon {
      display: block;
      width: 24px;
      height: 24px;
      fill: none;
      stroke: currentColor;
      stroke-width: 2;
      stroke-linecap: round;
      stroke-linejoin: round;
    }
    .long {
      display: none;
    }
    .short {
      display: block;
    }
    .label {
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 100%;
    }
    .badge {
      position: absolute;
      top: 0;
      left: calc(50% + 6px);
      margin: 0;
      font-size: 0.65rem;
      line-height: 1.4;
    }
  }
</style>
