import { useApp } from "../state/store.jsx";

export default function TabBar() {
    const { tabs, activeTab, selectTab, closeTab } = useApp();

    return (
        <div id="tabbar">
            {tabs.map((t) => (
                <div key={t.id} className={"tab" + (t.id === activeTab ? " active" : "")} onClick={() => selectTab(t.id)}>
                    <span className="tab-title" title={t.title}>
                        {t.title}
                    </span>
                    <button
                        className="tab-close"
                        onClick={(e) => {
                            e.stopPropagation();
                            closeTab(t.id);
                        }}
                    >
                        ✕
                    </button>
                </div>
            ))}
        </div>
    );
}
