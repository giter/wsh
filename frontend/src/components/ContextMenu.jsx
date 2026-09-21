import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

// ContextMenu is the right-click menu for the session tree (create / rename /
// delete). It renders at the pointer, portalled to <body> so the sidebar's
// scrolling and clipping cannot cut it off, and closes on Escape, on a click
// outside, and on resize/scroll, where the anchor point stops meaning anything.
//
// items is a list of { label, onClick, danger?, disabled? } entries, or
// { separator: true } to draw a divider.
export default function ContextMenu({ x, y, items, onClose }) {
    const ref = useRef(null);
    // Measured on mount: the menu is hidden for the first paint so it never
    // flashes at a position it would immediately be nudged away from.
    const [pos, setPos] = useState({ left: x, top: y, ready: false });

    useLayoutEffect(() => {
        const el = ref.current;
        if (!el) return;
        const { width, height } = el.getBoundingClientRect();
        const margin = 6;
        setPos({
            left: Math.max(margin, Math.min(x, window.innerWidth - width - margin)),
            top: Math.max(margin, Math.min(y, window.innerHeight - height - margin)),
            ready: true,
        });
    }, [x, y]);

    useEffect(() => {
        const onMouseDown = (e) => {
            if (ref.current && !ref.current.contains(e.target)) onClose();
        };
        const onKeyDown = (e) => {
            if (e.key === "Escape") onClose();
        };
        // Capture phase so the menu closes even if an inner handler stops
        // propagation, which the tree rows do.
        window.addEventListener("mousedown", onMouseDown, true);
        window.addEventListener("keydown", onKeyDown);
        window.addEventListener("resize", onClose);
        window.addEventListener("blur", onClose);
        window.addEventListener("wheel", onClose, { passive: true });
        return () => {
            window.removeEventListener("mousedown", onMouseDown, true);
            window.removeEventListener("keydown", onKeyDown);
            window.removeEventListener("resize", onClose);
            window.removeEventListener("blur", onClose);
            window.removeEventListener("wheel", onClose);
        };
    }, [onClose]);

    return createPortal(
        <div
            ref={ref}
            className="ctx-menu"
            role="menu"
            style={{ left: pos.left, top: pos.top, visibility: pos.ready ? "visible" : "hidden" }}
            onContextMenu={(e) => e.preventDefault()}
        >
            {items.map((it, i) =>
                it.separator ? (
                    // eslint-disable-next-line react/no-array-index-key
                    <div className="ctx-sep" key={`sep-${i}`} />
                ) : (
                    <button
                        key={it.label}
                        type="button"
                        role="menuitem"
                        className={"ctx-item" + (it.danger ? " danger" : "")}
                        disabled={it.disabled}
                        onClick={() => {
                            onClose();
                            it.onClick?.();
                        }}
                    >
                        {it.label}
                    </button>
                ),
            )}
        </div>,
        document.body,
    );
}
