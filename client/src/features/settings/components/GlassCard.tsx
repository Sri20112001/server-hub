export function GlassCard({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return (
    <section className={`bg-surface-container-lowest border border-outline-variant/20 rounded-2xl p-6 shadow-sm ${className}`}>
      {children}
    </section>
  );
}
