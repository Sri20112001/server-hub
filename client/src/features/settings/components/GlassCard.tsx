import React from 'react';

export function GlassCard({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return (
    <section className={`bg-white dark:bg-panel border border-line dark:border-edge rounded-card p-6 shadow-chrome ${className}`}>
      {children}
    </section>
  );
}
