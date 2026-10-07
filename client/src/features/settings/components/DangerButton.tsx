
export function DangerButton({ children, onClick, disabled, className = '' }: any) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`relative inline-flex items-center justify-center px-5 py-2.5 overflow-hidden font-medium text-red-50 transition-all duration-300 bg-red-950 rounded-lg group hover:bg-red-900 disabled:opacity-50 disabled:cursor-not-allowed border border-red-500/50 hover:border-red-400 shadow-[0_0_15px_rgba(239,68,68,0.2)] hover:shadow-[0_0_25px_rgba(239,68,68,0.6)] ${className}`}
    >
      <span className="relative z-10 flex items-center gap-2">{children}</span>
      <div className="absolute inset-0 h-full w-full bg-gradient-to-tr from-red-500/20 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
    </button>
  );
}
