package browser

import (
	"errors"
	"time"
)

// The page scripts and timings shared by the desktop browser (solver.go,
// build tag rod) and a browser driven by the host app (driven.go, the
// Android WebView).

const (
	headlessWait = 20 * time.Second // non-interactive challenges clear in a few seconds
	pollEvery    = 500 * time.Millisecond
)

// challengeJS reports whether the page is still a challenge interstitial
// (titles in en/pt/es).
const challengeJS = `() => typeof window._cf_chl_opt !== 'undefined' ||
  !!document.querySelector('#challenge-form, #challenge-stage, .cf-turnstile, #cf-challenge-running') ||
  /just a moment|um momento|un momento/i.test(document.title)`

// openChaptersJS clicks a "Chapters" tab when the page has one that is not
// active yet, so lists loaded on demand appear, and scrolls to the bottom to
// trigger lazy loading. It returns whether it clicked.
const openChaptersJS = `() => {
  window.scrollTo(0, document.body.scrollHeight);
  if (window.__mtTabClicked) return false;
  const re = /^\s*(chapters|cap[ií]tulos|episodes|epis[oó]dios)\s*(\(\s*\d+\s*\))?\s*$/i;
  const tabs = [...document.querySelectorAll('[role=tab], .tabs-item, .tab, button, li, a[href^="#"]')];
  const tab = tabs.find(el => re.test(el.textContent || ''));
  if (!tab || /active|selected/.test(tab.className) || tab.getAttribute('aria-selected') === 'true') return false;
  window.__mtTabClicked = true;
  tab.click();
  return true;
}`

// collectPagesJS walks a client-side pager (numbered <button>s that swap
// the list in place, never real links) and copies the links of every page
// into a hidden container at the top of the body, so the final HTML holds
// all of them in page order. It returns
// how many pages it visited.
const collectPagesJS = `async () => {
  const pagerSel = 'nav[aria-label*="agination" i], .pagination, .npager, .pager';
  const numbers = () => {
    const pager = document.querySelector(pagerSel);
    return pager ? [...pager.querySelectorAll('button')].filter(b => /^\d+$/.test(b.textContent.trim())) : [];
  };
  const labels = numbers().map(b => b.textContent.trim()).slice(0, 30);
  if (labels.length < 2) return 0;
  const acc = document.createElement('div');
  acc.id = 'mt-collected-pages';
  acc.hidden = true;
  const links = () => [...document.querySelectorAll('a[href]')].filter(a => !acc.contains(a));
  const copy = () => links().forEach(a => acc.appendChild(a.cloneNode(true)));
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  for (const label of labels) {
    const btn = numbers().find(b => b.textContent.trim() === label);
    if (!btn) continue;
    const before = links().map(a => a.href).join('|');
    if (!btn.matches('.is-active, .active, [aria-current=page]')) {
      btn.click();
      for (let i = 0; i < 20 && links().map(a => a.href).join('|') === before; i++) await sleep(250);
    }
    copy();
  }
  document.body.prepend(acc); // first in the document: page 1, 2, 3… in order
  return labels.length;
}`

// linkCountJS measures how much of the page has loaded.
const linkCountJS = `() => document.querySelectorAll('a[href]').length`

// postJS replays a POST from the page, so it carries the page's cookies.
const postJS = `async (url, body, type) => {
  const r = await fetch(url, {method: 'POST', body, credentials: 'include',
    headers: {'Content-Type': type, 'X-Requested-With': 'XMLHttpRequest'}});
  return {status: r.status, text: await r.text()};
}`

// errStillChallenged means the page was still a challenge when the wait
// ran out.
var errStillChallenged = errors.New("challenge not passed in time")
