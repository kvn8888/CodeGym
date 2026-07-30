import {
  CHAT_MARGIN,
  MIN_CHAT_H,
  MIN_CHAT_W,
  cornerPosition,
  maxChatSize,
  viewportSize,
} from './floatingChatGeometry';

function setViewport(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width });
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: height });
}

describe('floating chat geometry', () => {
  it('reads the active viewport and reserves the configured margin', () => {
    setViewport(1200, 800);

    expect(viewportSize()).toEqual({ width: 1200, height: 800 });
    expect(maxChatSize()).toEqual({
      width: 1200 - CHAT_MARGIN * 2,
      height: 800 - CHAT_MARGIN * 2,
    });
  });

  it('never reports a maximum below the minimum chat size', () => {
    setViewport(240, 300);

    expect(maxChatSize()).toEqual({ width: MIN_CHAT_W, height: MIN_CHAT_H });
  });

  it('anchors every corner inside the viewport', () => {
    setViewport(1000, 700);

    expect(cornerPosition('top-left', 300, 360)).toEqual({ x: 24, y: 24 });
    expect(cornerPosition('top-right', 300, 360)).toEqual({ x: 676, y: 24 });
    expect(cornerPosition('bottom-left', 300, 360)).toEqual({ x: 24, y: 316 });
    expect(cornerPosition('bottom-right', 300, 360)).toEqual({ x: 676, y: 316 });
  });
});
