// Single-box dual-track parsing helpers.
//
// The bottom Smart Input takes both shell commands and natural language, so it
// has to decide which one it is looking at. Parsing alone is not enough: a POSIX
// parser happily reads "帮我找出占用 8080 端口的进程" as a call to the command
// "帮我找出占用", so the decision combines three signals (see PLAN.md §1.3.3):
//
//  1. an explicit mode toggle wins outright (Auto / Shell / AI);
//  2. CJK text is never shell;
//  3. otherwise, text the shell parser rejects is treated as natural language.

// CJK covers Han, Hiragana/Katakana and Hangul, which never appear in a command.
const CJK = /[\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\uff66-\uff9f\uac00-\ud7af]/;

// QUESTION_WORDS are the English phrasings that read as intent rather than as a
// command. Kept narrow on purpose: "ls" must not look like a question.
const QUESTION_WORDS = /^(how|what|why|where|which|who|can you|could you|please|explain|help me|show me|find me|list all)\b/i;

// looksLikeNaturalLanguage decides whether text should go through the AI track.
export function looksLikeNaturalLanguage(text, risk) {
    const s = String(text || "").trim();
    if (!s) return false;
    if (CJK.test(s)) return true;
    if (QUESTION_WORDS.test(s)) return true;
    // The shell parser rejected it, so it is not something to run verbatim.
    if (risk && risk.parseError) return true;
    return false;
}

// riskLabel renders a safety level for the UI.
export function riskLabel(level) {
    switch (level) {
        case "blocked":
            return "红区 · 已阻断";
        case "caution":
            return "黄区 · 需确认";
        default:
            return "绿区 · 安全";
    }
}

// sevLabel renders a sniffer severity.
export function sevLabel(severity) {
    switch (severity) {
        case "high":
            return "高危";
        case "medium":
            return "中等";
        default:
            return "提示";
    }
}
