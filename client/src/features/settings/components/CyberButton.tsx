import React from 'react';

type CyberButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement>;

export function CyberButton({ children, onClick, disabled, type = 'button', className = '' }: CyberButtonProps) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center justify-center gap-2 px-4 py-2 font-medium text-[13px] text-white dark:text-black transition-colors duration-150 bg-accent hover:bg-accent-hover dark:bg-ember dark:hover:bg-ember-hover rounded-input disabled:opacity-50 disabled:cursor-not-allowed shadow-sm ${className}`}
    >
      {children}
    </button>
  );
}
