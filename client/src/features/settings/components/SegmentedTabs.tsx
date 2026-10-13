import React from 'react';

export interface Tab {
  id: string;
  label: string;
  icon?: React.ReactNode;
}

export function SegmentedTabs({ tabs, activeId, onChange }: { tabs: Tab[]; activeId: string; onChange: (id: string) => void }) {
  return (
    <div className="flex items-center overflow-x-auto p-1 bg-surface-container rounded-full max-w-fit shadow-inner">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onChange(tab.id)}
          className={`flex items-center justify-center gap-2 py-1.5 px-4 text-[13px] font-medium rounded-full transition-all cursor-pointer border-0 whitespace-nowrap ${
            activeId === tab.id
              ? 'bg-surface-container-lowest text-primary font-semibold shadow-sm'
              : 'text-on-surface-variant hover:text-on-surface hover:bg-surface-container-high'
          }`}
          type="button"
        >
          {tab.icon}
          <span>{tab.label}</span>
        </button>
      ))}
    </div>
  );
}
