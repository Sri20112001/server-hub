import React from 'react';

type DangerButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement>;

export function DangerButton({ children, onClick, disabled, className = '' }: DangerButtonProps) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center justify-center gap-2 px-4 py-2 font-medium text-[13px] text-brick transition-colors duration-150 bg-transparent hover:bg-red-50 dark:hover:bg-red-950/40 rounded-input disabled:opacity-50 disabled:cursor-not-allowed border border-brick/40 hover:border-brick ${className}`}
    >
      {children}
    </button>
  );
}
