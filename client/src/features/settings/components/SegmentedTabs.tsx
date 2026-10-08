import React from 'react';

export interface Tab {
  id: string;
  label: string;
  icon?: React.ReactNode;
}

export function SegmentedTabs({ tabs, activeId, onChange }: { tabs: Tab[]; activeId: string; onChange: (id: string) => void }) {
  return (
    <div className="flex p-1 space-x-1 bg-paper dark:bg-abyss rounded-xl border border-line dark:border-edge w-full">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onChange(tab.id)}
          className={`flex-1 flex items-center justify-center gap-2 py-2 px-4 text-sm font-semibold rounded-lg transition-colors duration-150 ${
            activeId === tab.id
              ? 'bg-white dark:bg-panel text-ink dark:text-bone shadow-sm border border-line dark:border-edge'
              : 'text-muted dark:text-fog hover:text-ink dark:hover:text-bone hover:bg-white/40 dark:hover:bg-emboss/40 border border-transparent'
          }`}
        >
          {tab.icon}
          {tab.label}
        </button>
      ))}
    </div>
  );
}
