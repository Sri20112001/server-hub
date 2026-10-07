import React from 'react';

type CyberButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement>;

export function CyberButton({ children, onClick, disabled, type = 'button', className = '' }: CyberButtonProps) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`relative inline-flex items-center justify-center px-5 py-2.5 overflow-hidden font-medium text-white transition-all duration-300 bg-gray-800 dark:bg-gray-900 rounded-lg group hover:bg-gray-700 dark:hover:bg-gray-800 disabled:opacity-50 disabled:cursor-not-allowed border border-cyan-500/40 hover:border-cyan-400 shadow-[0_0_15px_rgba(6,182,212,0.2)] hover:shadow-[0_0_25px_rgba(6,182,212,0.5)] ${className}`}
    >
      <span className="relative z-10 flex items-center gap-2">{children}</span>
      <div className="absolute inset-0 h-full w-full bg-gradient-to-tr from-cyan-500/10 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />
    </button>
  );
}
