import { api } from './api';

// Saves the library export (RF-43) the way each front end can:
// - desktop app (Wails): the server opens a native "save as" dialog;
// - Android app: its bridge opens the system picker to choose a folder and
//   answers with a "mt-file-saved" event;
// - browser: a regular download.

export type ExportResult =
  { kind: 'saved'; file: string } | { kind: 'downloaded'; file: string } | { kind: 'canceled' };

interface AndroidBridge {
  saveFile(name: string, content: string): void;
}

interface SavedEvent {
  ok: boolean;
  canceled?: boolean;
  name?: string;
  error?: string;
}

function androidBridge(): AndroidBridge | undefined {
  const b = (window as { LeftOffAndroid?: Partial<AndroidBridge> }).LeftOffAndroid;
  return typeof b?.saveFile === 'function' ? (b as AndroidBridge) : undefined;
}

/** Waits for the Android app to say how the save went. */
function androidSave(bridge: AndroidBridge, name: string, content: string): Promise<ExportResult> {
  return new Promise((resolve, reject) => {
    const done = (e: Event) => {
      window.removeEventListener('mt-file-saved', done);
      const d = (e as CustomEvent<SavedEvent>).detail;
      if (d.canceled) resolve({ kind: 'canceled' });
      else if (d.ok) resolve({ kind: 'saved', file: d.name || name });
      else reject(new Error(d.error || 'save failed'));
    };
    window.addEventListener('mt-file-saved', done);
    bridge.saveFile(name, content);
  });
}

export async function exportLibrary(mode: string): Promise<ExportResult> {
  if (mode === 'desktop') {
    const r = await api.saveExportDesktop();
    return r.canceled ? { kind: 'canceled' } : { kind: 'saved', file: r.path ?? '' };
  }
  const { blob, filename } = await api.exportLibrary();
  const android = androidBridge();
  if (android) return androidSave(android, filename, await blob.text());
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
  return { kind: 'downloaded', file: filename };
}
