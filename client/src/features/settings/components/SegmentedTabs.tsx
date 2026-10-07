import React from 'react';

export interface Tab {
  id: string;
  label: string;
  icon?: React.ReactNode;
}

export function SegmentedTabs({ tabs, activeId, onChange }: { tabs: Tab[]; activeId: string; onChange: (id: string) => void }) {
  return (
    <div className="flex p-1.5 space-x-1.5 bg-gray-200/50 dark:bg-black/40 rounded-2xl backdrop-blur-md border border-gray-300/50 dark:border-white/10 w-full">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onChange(tab.id)}
          className={`flex-1 flex items-center justify-center gap-2 py-2 px-4 text-sm font-semibold rounded-xl transition-all duration-300 ${
            activeId === tab.id
              ? 'bg-white dark:bg-white/15 text-gray-900 dark:text-cyan-400 shadow-sm dark:shadow-[0_0_15px_rgba(6,182,212,0.3)] border border-transparent dark:border-cyan-500/30'
              : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:bg-white/50 dark:hover:bg-white/5 border border-transparent'
          }`}
        >
          {tab.icon}
          {tab.label}
        </button>
      ))}
    </div>
  );
}
