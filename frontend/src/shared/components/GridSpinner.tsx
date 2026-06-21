import { useMemo } from 'react';

interface GridSpinnerProps {
  size?: 'sm' | 'md' | 'lg';
}

/**
 * Generates a clockwise spiral sequence of (row, col) coordinates
 * for a 5x5 grid, starting from the top-left of the outermost ring
 * and spiraling inward to the center cell.
 */
function getClockwiseSequence(): [number, number][] {
  const sequence: [number, number][] = [];
  const n = 5;

  for (let ring = 0; ring <= Math.floor(n / 2); ring++) {
    const start = ring;
    const end = n - 1 - ring;

    if (start === end) {
      sequence.push([start, start]);
      break;
    }

    // Top edge: left to right
    for (let col = start; col <= end; col++) sequence.push([start, col]);
    // Right edge: top+1 to bottom
    for (let row = start + 1; row <= end; row++) sequence.push([row, end]);
    // Bottom edge: right-1 to left
    for (let col = end - 1; col >= start; col--) sequence.push([end, col]);
    // Left edge: bottom-1 to top+1
    for (let row = end - 1; row > start; row--) sequence.push([row, start]);
  }

  return sequence;
}

const SEQUENCE = getClockwiseSequence();

// Map (row,col) → sequence index for animation delay
const DELAY_MAP = new Map<string, number>();
SEQUENCE.forEach(([r, c], i) => {
  DELAY_MAP.set(`${r},${c}`, i);
});

const SIZES = {
  sm: { cell: 4, gap: 2 },
  md: { cell: 7, gap: 3 },
  lg: { cell: 10, gap: 4 },
};

const TOTAL_CELLS = 25;
const TOTAL_DURATION = 2.5; // seconds
const STAGGER = (TOTAL_DURATION * 0.7) / TOTAL_CELLS;

export function GridSpinner({ size = 'md' }: GridSpinnerProps) {
  const { cell, gap } = SIZES[size];

  const cells = useMemo(() => {
    const result = [];
    for (let row = 0; row < 5; row++) {
      for (let col = 0; col < 5; col++) {
        const seqIndex = DELAY_MAP.get(`${row},${col}`) ?? 0;
        result.push({ row, col, delay: seqIndex * STAGGER });
      }
    }
    return result;
  }, []);

  const gridSize = 5 * cell + 4 * gap;

  return (
    <div
      role="status"
      aria-label="Loading"
      style={{
        display: 'inline-grid',
        gridTemplateColumns: `repeat(5, ${cell}px)`,
        gap: `${gap}px`,
        width: gridSize,
        height: gridSize,
      }}
    >
      {cells.map(({ row, col, delay }) => (
        <div
          key={`${row}-${col}`}
          style={{
            width: cell,
            height: cell,
            backgroundColor: 'var(--color-gray-200)',
            animation: `grid-blink ${TOTAL_DURATION}s ${delay}s infinite`,
          }}
        />
      ))}
    </div>
  );
}
