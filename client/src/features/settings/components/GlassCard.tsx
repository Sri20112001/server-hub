import React from 'react';

export function GlassCard({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return (
    <section className={`bg-white/60 dark:bg-gray-900/40 backdrop-blur-xl border border-white/20 dark:border-white/10 rounded-2xl p-6 shadow-[0_8px_32px_0_rgba(0,0,0,0.05)] dark:shadow-[0_8px_32px_0_rgba(0,0,0,0.3)] relative overflow-hidden ${className}`}>
      {/* Subtle glow effect */}
      <div className="absolute top-0 left-0 w-full h-full pointer-events-none bg-gradient-to-br from-white/10 to-transparent dark:from-white/5 opacity-50"></div>
      <div className="relative z-10">{children}</div>
    </section>
  );
}
