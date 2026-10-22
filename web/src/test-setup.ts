import '@testing-library/jest-dom/vitest';

// Tests assert pt-BR texts; jsdom reports an English browser.
localStorage.setItem('mt.lang', 'pt-BR');
