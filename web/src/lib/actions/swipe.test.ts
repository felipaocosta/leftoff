import { afterEach, describe, expect, it, vi } from 'vitest';
import { swipe } from './swipe';

// jsdom has no PointerEvent: a MouseEvent with the pointer fields is enough.
function pointer(node: HTMLElement, type: string, x: number, y = 0, pointerType = 'touch') {
  const e = new MouseEvent(type, { clientX: x, clientY: y, bubbles: true });
  Object.assign(e, { pointerType, isPrimary: true });
  node.dispatchEvent(e);
}

describe('swipe', () => {
  afterEach(() => vi.useRealTimers());

  it('calls onSwipe after a long enough sideways move', () => {
    const node = document.createElement('li');
    const onSwipe = vi.fn();
    swipe(node, { onSwipe });
    pointer(node, 'pointerdown', 100);
    pointer(node, 'pointermove', 140);
    expect(node.style.getPropertyValue('--swipe-x')).toBe('40px');
    pointer(node, 'pointermove', 190);
    pointer(node, 'pointerup', 190);
    expect(onSwipe).toHaveBeenCalledOnce();
    expect(node.style.getPropertyValue('--swipe-x')).toBe('0px');

    pointer(node, 'pointerdown', 100);
    pointer(node, 'pointermove', 130);
    pointer(node, 'pointerup', 130); // too short
    pointer(node, 'pointerdown', 100, 0);
    pointer(node, 'pointermove', 105, 60); // a vertical scroll
    pointer(node, 'pointerup', 200, 60);
    pointer(node, 'pointerdown', 100, 0, 'mouse'); // mice click instead
    pointer(node, 'pointerup', 300, 0, 'mouse');
    expect(onSwipe).toHaveBeenCalledOnce();
  });

  it('calls onLongPress after holding still and swallows the click', () => {
    vi.useFakeTimers();
    const node = document.createElement('li');
    const onLongPress = vi.fn();
    swipe(node, { onLongPress });
    pointer(node, 'pointerdown', 100);
    vi.advanceTimersByTime(600);
    pointer(node, 'pointerup', 100);
    expect(onLongPress).toHaveBeenCalledOnce();
    const click = new MouseEvent('click', { bubbles: true, cancelable: true });
    node.dispatchEvent(click);
    expect(click.defaultPrevented).toBe(true);

    pointer(node, 'pointerdown', 100);
    pointer(node, 'pointermove', 100, 30); // moved: not a long press
    vi.advanceTimersByTime(600);
    expect(onLongPress).toHaveBeenCalledOnce();
  });
});
