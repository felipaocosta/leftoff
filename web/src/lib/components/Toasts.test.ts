import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Toasts from './Toasts.svelte';
import { toasts } from '../stores/toast.svelte';

describe('Toasts', () => {
  afterEach(() => vi.useRealTimers());

  it('shows messages, hides them after a while and on dismiss', async () => {
    vi.useFakeTimers();
    render(Toasts);
    toasts.show('Salvo');
    toasts.show('Falhou', 'error');
    await vi.advanceTimersByTimeAsync(0);
    expect(screen.getByRole('status')).toHaveTextContent('Salvo');
    expect(screen.getByRole('alert')).toHaveTextContent('Falhou');
    const [, closeError] = screen.getAllByRole('button', { name: 'Fechar aviso' });
    if (closeError) await fireEvent.click(closeError);
    expect(screen.queryByRole('alert')).toBeNull();
    await vi.advanceTimersByTimeAsync(5000);
    expect(screen.queryByRole('status')).toBeNull();
  });
});
