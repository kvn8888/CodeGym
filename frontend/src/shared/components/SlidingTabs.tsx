import { useCallback, useLayoutEffect, useRef } from 'react';

/**
 * SlidingTabs — a segmented control whose active pill slides between options.
 *
 * Implements the transitions-dev "Tabs sliding" transition (16): JS writes the
 * active tab's offsetLeft / offsetWidth onto the pill and CSS owns the tween.
 * The pill is snapped into place WITHOUT a transition on first paint and on
 * resize (otherwise it animates in from translateX(0) / width: 0), then tweens
 * on every subsequent selection. The `.t-tabs`, `.t-tab`, and `.t-tabs-pill`
 * styles live in index.css.
 */

export interface SlidingTabsOption<T extends string> {
  value: T;
  label: string;
}

interface SlidingTabsProps<T extends string> {
  options: SlidingTabsOption<T>[];
  value: T;
  onChange: (value: T) => void;
  ariaLabel?: string;
  className?: string;
}

export function SlidingTabs<T extends string>({
  options,
  value,
  onChange,
  ariaLabel,
  className,
}: SlidingTabsProps<T>) {
  const pillRef = useRef<HTMLSpanElement>(null);
  const tabRefs = useRef(new Map<T, HTMLButtonElement>());
  // First paint must snap (the pill starts at width: 0); only later selections tween.
  const firstPaint = useRef(true);

  const positionPill = useCallback(
    (animate: boolean) => {
      const pill = pillRef.current;
      const activeTab = tabRefs.current.get(value);
      if (!pill || !activeTab) return;

      if (animate) {
        pill.style.transform = `translateX(${activeTab.offsetLeft}px)`;
        pill.style.width = `${activeTab.offsetWidth}px`;
      } else {
        // Suspend the transition, write the values, force a reflow, restore.
        const prev = pill.style.transition;
        pill.style.transition = 'none';
        pill.style.transform = `translateX(${activeTab.offsetLeft}px)`;
        pill.style.width = `${activeTab.offsetWidth}px`;
        void pill.offsetWidth;
        pill.style.transition = prev;
      }
    },
    [value],
  );

  // Place / move the pill whenever the selection (or option set) changes.
  useLayoutEffect(() => {
    positionPill(!firstPaint.current);
    firstPaint.current = false;
  }, [positionPill, options]);

  // Keep the pill aligned when layout reflows (e.g. viewport resize). No tween.
  useLayoutEffect(() => {
    const handleResize = () => positionPill(false);
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [positionPill]);

  return (
    <div role="tablist" aria-label={ariaLabel} className={`t-tabs ${className ?? ''}`}>
      <span ref={pillRef} className="t-tabs-pill" aria-hidden="true" />
      {options.map((opt) => (
        <button
          key={opt.value}
          ref={(el) => {
            if (el) tabRefs.current.set(opt.value, el);
            else tabRefs.current.delete(opt.value);
          }}
          type="button"
          role="tab"
          aria-selected={value === opt.value}
          onClick={() => onChange(opt.value)}
          className="t-tab cg-focus text-sm"
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}
