export type AnchorCorner = 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';

export const CHAT_MARGIN = 24;
export const BUBBLE_SIZE = 56;
export const DEFAULT_CHAT_W = 380;
export const DEFAULT_CHAT_H = 520;
export const MIN_CHAT_W = 300;
export const MIN_CHAT_H = 360;
const WINDOW_ANCHORS: AnchorCorner[] = ['top-right', 'top-left', 'bottom-left', 'bottom-right'];

export interface ChatBounds {
  x: number;
  y: number;
  width: number;
  height: number;
}

export function viewportSize() {
  return {
    width: window.innerWidth,
    height: window.innerHeight,
  };
}

export function maxChatSize(margin = CHAT_MARGIN) {
  const { width, height } = viewportSize();
  return {
    width: Math.max(MIN_CHAT_W, width - margin * 2),
    height: Math.max(MIN_CHAT_H, height - margin * 2),
  };
}

export function cornerPosition(
  corner: AnchorCorner,
  width: number,
  height: number,
  margin = CHAT_MARGIN,
): { x: number; y: number } {
  const { width: vw, height: vh } = viewportSize();
  const left = Math.min(margin, Math.max(0, vw - width));
  const right = Math.max(left, vw - width - margin);
  const top = Math.min(margin, Math.max(0, vh - height));
  const bottom = Math.max(top, vh - height - margin);

  switch (corner) {
    case 'top-left':
      return { x: left, y: top };
    case 'top-right':
      return { x: right, y: top };
    case 'bottom-left':
      return { x: left, y: bottom };
    case 'bottom-right':
      return { x: right, y: bottom };
  }
}

export function nearestCorner(
  x: number,
  y: number,
  width: number,
  height: number,
  margin = CHAT_MARGIN,
  corners: readonly AnchorCorner[] = WINDOW_ANCHORS,
): AnchorCorner {
  let best = corners[0];
  let bestDistance = Infinity;

  for (const corner of corners) {
    const anchor = cornerPosition(corner, width, height, margin);
    const distance = (x - anchor.x) ** 2 + (y - anchor.y) ** 2;
    if (distance < bestDistance) {
      bestDistance = distance;
      best = corner;
    }
  }

  return best;
}

export function snapToNearestCorner(bounds: ChatBounds, margin = CHAT_MARGIN): ChatBounds {
  const max = maxChatSize(margin);
  const width = Math.min(Math.max(bounds.width, MIN_CHAT_W), max.width);
  const height = Math.min(Math.max(bounds.height, MIN_CHAT_H), max.height);
  const corner = nearestCorner(bounds.x, bounds.y, width, height, margin);
  const anchor = cornerPosition(corner, width, height, margin);
  return { x: anchor.x, y: anchor.y, width, height };
}

export function defaultOpenBounds(): ChatBounds {
  const max = maxChatSize();
  const width = Math.min(DEFAULT_CHAT_W, max.width);
  const height = Math.min(DEFAULT_CHAT_H, max.height);
  const anchor = cornerPosition('top-right', width, height);
  return { x: anchor.x, y: anchor.y, width, height };
}

export function bubbleBounds(): ChatBounds {
  const anchor = cornerPosition('bottom-right', BUBBLE_SIZE, BUBBLE_SIZE);
  return { x: anchor.x, y: anchor.y, width: BUBBLE_SIZE, height: BUBBLE_SIZE };
}

const STORAGE_KEY = 'codegym.chat.bounds';

export function loadStoredBounds(): ChatBounds | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as ChatBounds;
    if (
      typeof parsed.x === 'number' &&
      typeof parsed.y === 'number' &&
      typeof parsed.width === 'number' &&
      typeof parsed.height === 'number'
    ) {
      return parsed;
    }
  } catch {
    // ignore corrupt storage
  }
  return null;
}

export function saveStoredBounds(bounds: ChatBounds) {
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(bounds));
}
