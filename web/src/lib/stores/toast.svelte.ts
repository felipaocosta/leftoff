// Short floating messages ("snackbar") for results the user must notice,
// such as an import or an export, wherever the page is scrolled.

export type ToastKind = 'ok' | 'error';

export interface Toast {
  id: number;
  text: string;
  kind: ToastKind;
}

const SHOW_MS = 5000;

class Toasts {
  items = $state<Toast[]>([]);
  private seq = 0;

  show(text: string, kind: ToastKind = 'ok'): void {
    const id = ++this.seq;
    this.items = [...this.items, { id, text, kind }];
    setTimeout(() => this.dismiss(id), SHOW_MS);
  }

  dismiss(id: number): void {
    this.items = this.items.filter((t) => t.id !== id);
  }
}

export const toasts = new Toasts();
