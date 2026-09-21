export default function Modal({ title, children, actions, onClose }) {
    return (
        <div
            className="modal-backdrop"
            onMouseDown={(e) => {
                if (e.target === e.currentTarget) onClose?.();
            }}
        >
            <div className="modal">
                <h3>{title}</h3>
                {children}
                {actions && <div className="modal-actions">{actions}</div>}
            </div>
        </div>
    );
}
