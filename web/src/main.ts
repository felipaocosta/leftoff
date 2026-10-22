import { mount } from 'svelte';
import App from './App.svelte';
import './app.css';

const target = document.getElementById('app');
if (!target) throw new Error('missing #app element');

export default mount(App, { target });

// PWA: offline reading and "install app". Browsers only allow service
// workers in secure contexts (HTTPS or localhost).
if ('serviceWorker' in navigator && window.isSecureContext) {
  navigator.serviceWorker.register('./sw.js').catch(() => {
    // Not fatal: the app works online without it.
  });
}
