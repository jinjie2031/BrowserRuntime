import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import en from './en.json';
import zh from './zh-CN.json';

export type Language = 'en-US' | 'zh-CN';
export const languagePreference = 'browserruntime.ui.language';
export const dictionaries: Record<Language, Record<string, string>> = { 'en-US': en, 'zh-CN': zh };

export function translate(language: Language, key: string, values?: Record<string, string>): string {
  const dictionary = dictionaries[language];
  const template = Object.hasOwn(dictionary, key) ? dictionary[key] : dictionary['The operation could not be completed.'];
  return template.replace(/\{(\w+)\}/g, (placeholder, name: string) => values?.[name] ?? placeholder);
}

// Only known status labels are translated; technical and user-supplied values stay intact.
const statusLabels = new Set(['Direct', 'Proxy', 'Ready', 'Needs Test', 'Connection Failed', 'Configuration Error', 'Running', 'Stopped', 'Starting', 'Stopping', 'ExternalRunning', 'InUse', 'Unknown']);
function readLanguage(): Language {
  try { return localStorage.getItem(languagePreference) === 'zh-CN' ? 'zh-CN' : 'en-US'; }
  catch { return 'en-US'; }
}

async function syncNativeLanguage(language: Language) {
  const host = window as unknown as { go?: { main?: { UILanguage?: { SetLanguage(value: Language): Promise<void> } } } };
  // Presentation-only binding, no Profile/Network methods or persistence.
  try { await host.go?.main?.UILanguage?.SetLanguage(language); } catch { /* No raw diagnostic data. */ }
}

const LanguageContext = createContext({ language: 'en-US' as Language, toggle: () => {}, t: (key: string, values?: Record<string, string>) => translate('en-US', key, values), label: (value: string) => value });

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguage] = useState<Language>(readLanguage);
  useEffect(() => { document.documentElement.lang = language; void syncNativeLanguage(language); }, [language]);
  function toggle() {
    const next = language === 'en-US' ? 'zh-CN' : 'en-US';
    try { localStorage.setItem(languagePreference, next); } catch { /* Switching still works without storage. */ }
    setLanguage(next);
  }
  const t = (key: string, values?: Record<string, string>) => translate(language, key, values);
  const label = (value: string) => statusLabels.has(value) ? t(value) : value;
  return <LanguageContext.Provider value={{ language, toggle, t, label }}>{children}</LanguageContext.Provider>;
}

export function useLanguage() { return useContext(LanguageContext); }
