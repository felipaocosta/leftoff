import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import ContinueButton from './ContinueButton.svelte';
import { makeSeries } from '../test-fixtures';

describe('ContinueButton', () => {
  it('opens the next unread chapter and keeps room for itself while shown', () => {
    const chapter = {
      key: '101',
      number: 101,
      label: 'Chapter 101',
      url: 'https://site.org/c/101',
      alt_urls: null,
      released_at: null,
      discovered_at: null,
      read: false,
      read_at: null,
    };
    const { unmount } = render(ContinueButton, { series: makeSeries({ chapters: [chapter] }) });
    expect(screen.getByRole('link', { name: 'Continuar: Chapter 101' })).toHaveAttribute(
      'href',
      'https://site.org/c/101',
    );
    expect(document.body.classList.contains('has-sticky-action')).toBe(true);
    unmount();
    expect(document.body.classList.contains('has-sticky-action')).toBe(false);
  });
});
