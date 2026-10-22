import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Spinner from './Spinner.svelte';

describe('Spinner', () => {
  it('announces its label as a status', () => {
    render(Spinner, { label: 'Carregando…' });
    expect(screen.getByRole('status')).toHaveTextContent('Carregando…');
  });

  it('is decorative without a label', () => {
    const { container } = render(Spinner);
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(container.querySelector('.spinner')).toHaveAttribute('aria-hidden', 'true');
  });
});
