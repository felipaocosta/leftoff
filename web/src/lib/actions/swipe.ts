// Touch gestures for list rows: a horizontal swipe and a long press.
// Pointer events with touch-action: pan-y keep vertical scrolling native;
// the row follows the finger while it swipes (--swipe-x).

export interface SwipeOptions {
  onSwipe?: () => void;
  onLongPress?: () => void;
}

const SWIPE_PX = 64; // horizontal distance that counts as a swipe
const SLOP_PX = 10; // movement that cancels a long press
const LONG_PRESS_MS = 550;

/** Short haptic feedback where the device supports it. */
export function buzz(): void {
  navigator.vibrate?.(15);
}

export function swipe(node: HTMLElement, options: SwipeOptions) {
  let opts = options;
  let startX = 0;
  let startY = 0;
  let tracking = false;
  let swiping = false;
  let longTimer: ReturnType<typeof setTimeout> | undefined;
  let longFired = false;

  node.style.touchAction = 'pan-y';

  function reset() {
    clearTimeout(longTimer);
    tracking = swiping = false;
    node.style.setProperty('--swipe-x', '0px');
    node.classList.remove('swiping');
  }

  function down(e: PointerEvent) {
    if (e.pointerType === 'mouse' || !e.isPrimary) return;
    tracking = true;
    longFired = false;
    startX = e.clientX;
    startY = e.clientY;
    longTimer = setTimeout(() => {
      if (!tracking || swiping) return;
      longFired = true;
      buzz();
      opts.onLongPress?.();
    }, LONG_PRESS_MS);
  }

  function move(e: PointerEvent) {
    if (!tracking) return;
    const dx = e.clientX - startX;
    const dy = e.clientY - startY;
    if (Math.abs(dx) > SLOP_PX || Math.abs(dy) > SLOP_PX) clearTimeout(longTimer);
    if (!swiping && Math.abs(dy) > Math.abs(dx)) {
      reset(); // a vertical scroll
      return;
    }
    if (Math.abs(dx) > SLOP_PX) {
      swiping = true;
      node.classList.add('swiping');
      node.style.setProperty('--swipe-x', `${Math.max(-120, Math.min(120, dx))}px`);
    }
  }

  function up(e: PointerEvent) {
    if (!tracking) return;
    const dx = e.clientX - startX;
    const done = swiping && Math.abs(dx) >= SWIPE_PX;
    reset();
    if (done) {
      buzz();
      opts.onSwipe?.();
    }
  }

  // A long press must not also follow the link under the finger.
  function click(e: MouseEvent) {
    if (longFired) {
      e.preventDefault();
      e.stopPropagation();
      longFired = false;
    }
  }

  node.addEventListener('pointerdown', down);
  node.addEventListener('pointermove', move);
  node.addEventListener('pointerup', up);
  node.addEventListener('pointercancel', reset);
  node.addEventListener('click', click, true);
  node.addEventListener('contextmenu', (e) => longFired && e.preventDefault());
  return {
    update(next: SwipeOptions) {
      opts = next;
    },
    destroy() {
      clearTimeout(longTimer);
      node.removeEventListener('pointerdown', down);
      node.removeEventListener('pointermove', move);
      node.removeEventListener('pointerup', up);
      node.removeEventListener('pointercancel', reset);
      node.removeEventListener('click', click, true);
    },
  };
}
