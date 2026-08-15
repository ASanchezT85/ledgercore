"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import {
  DICTIONARIES,
  detectLang,
  LANG_STORAGE_KEY,
  type Dictionary,
  type Lang,
} from "@/lib/i18n";

type LanguageContextValue = {
  lang: Lang;
  t: Dictionary;
  setLang: (lang: Lang) => void;
};

const LanguageContext = createContext<LanguageContextValue | null>(null);

export function LanguageProvider({ children }: { children: React.ReactNode }) {
  // SSR y primer render del cliente arrancan en EN (fallback global);
  // el efecto aplica localStorage / navigator.language sin desajuste de hidratación.
  const [lang, setLangState] = useState<Lang>("en");

  useEffect(() => {
    setLangState(detectLang());
  }, []);

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    try {
      window.localStorage.setItem(LANG_STORAGE_KEY, next);
    } catch {
      /* localStorage bloqueado */
    }
    document.documentElement.lang = next;
  }, []);

  useEffect(() => {
    document.documentElement.lang = lang;
  }, [lang]);

  return (
    <LanguageContext.Provider value={{ lang, t: DICTIONARIES[lang], setLang }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLang(): LanguageContextValue {
  const ctx = useContext(LanguageContext);
  if (!ctx) throw new Error("useLang must be used within <LanguageProvider>");
  return ctx;
}

export function LangToggle() {
  const { lang, setLang } = useLang();
  return (
    <div
      className="inline-flex items-center rounded-(--radius-control) border border-edge-strong p-0.5"
      role="group"
      aria-label="Language / Idioma"
    >
      {(["es", "en"] as const).map((code) => (
        <button
          key={code}
          type="button"
          onClick={() => setLang(code)}
          aria-pressed={lang === code}
          className={
            "inline-flex h-9 items-center rounded-[calc(var(--radius-control)-2px)] px-3 text-xs font-semibold uppercase transition-colors " +
            (lang === code
              ? "bg-accent-soft text-accent"
              : "text-ink-faint hover:text-ink")
          }
        >
          {code}
        </button>
      ))}
    </div>
  );
}
