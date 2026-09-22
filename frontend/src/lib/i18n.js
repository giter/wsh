// Lightweight i18n runtime. Dictionaries live in ./i18n/*.js files, each
// exporting a default object of the shape:
//
//   export default {
//     en: { "common.cancel": "Cancel", ... },
//     zh: { "common.cancel": "取消", ... },
//   };
//
// Files are merged eagerly at startup, so adding a feature's strings is just a
// matter of dropping a new file in this directory — nothing else to wire up.
import { useSyncExternalStore } from "react";

const modules = import.meta.glob("./i18n/*.js", { eager: true });
const dicts = {};
for (const mod of Object.values(modules)) {
    for (const [lang, table] of Object.entries(mod.default || {})) {
        dicts[lang] = { ...(dicts[lang] || {}), ...table };
    }
}

// Languages offered in the options window. "auto" follows the webview locale.
export const LANGUAGES = [
    { value: "auto", label: "Auto" },
    { value: "en", label: "English" },
    { value: "zh", label: "中文" },
];

export const DEFAULT_LANGUAGE = "en";
let current = DEFAULT_LANGUAGE;
const listeners = new Set();

// resolveLanguage maps a stored preference ("auto"/"en"/"zh"/"") to an
// available dictionary, falling back to the webview locale for "auto".
export function resolveLanguage(pref) {
    if (pref && pref !== "auto" && dicts[pref]) return pref;
    const nav = (typeof navigator !== "undefined" && navigator.language) || "";
    if (/^zh\b|-?CN/i.test(nav) && dicts.zh) return "zh";
    return DEFAULT_LANGUAGE in dicts || dicts[DEFAULT_LANGUAGE] ? DEFAULT_LANGUAGE : "en";
}

// setLanguage applies a preference and notifies useT() subscribers. Called
// whenever settings change (the preference is persisted as settings.language).
export function setLanguage(pref) {
    const next = resolveLanguage(pref);
    if (next === current) return;
    current = next;
    for (const fn of listeners) fn();
}

export function currentLanguage() {
    return current;
}

// t looks up a key in the active dictionary, then English, then returns the key
// itself (so a missing translation is visible instead of rendering blank).
// Positional values are interpolated as {name}.
export function t(key, params) {
    let s = dicts[current]?.[key] ?? dicts[DEFAULT_LANGUAGE]?.[key] ?? key;
    if (params) {
        for (const [k, v] of Object.entries(params)) {
            s = s.replaceAll(`{${k}}`, String(v));
        }
    }
    return s;
}

function subscribe(fn) {
    listeners.add(fn);
    return () => listeners.delete(fn);
}

// useT returns the translate function and re-renders the component when the
// language changes.
export function useT() {
    useSyncExternalStore(subscribe, currentLanguage);
    return t;
}
