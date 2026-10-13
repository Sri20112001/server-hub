import React from 'react';

type DangerButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement>;

export function DangerButton({ children, onClick, disabled, className = '' }: DangerButtonProps) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center justify-center gap-2 px-5 py-2 font-semibold text-[13px] text-error transition-colors duration-150 bg-error-container/10 hover:bg-error-container/25 rounded-full disabled:opacity-50 disabled:cursor-not-allowed border border-error/30 hover:border-error/50 shadow-sm ${className}`}
    >
      {children}
    </button>
  );
}
