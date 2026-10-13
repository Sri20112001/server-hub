import React from 'react';

type CyberButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement>;

export function CyberButton({ children, onClick, disabled, type = 'button', className = '' }: CyberButtonProps) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center justify-center gap-2 px-5 py-2 font-semibold text-[13px] text-on-primary transition-all duration-150 bg-primary hover:bg-primary/90 rounded-full disabled:opacity-50 disabled:cursor-not-allowed shadow-sm ${className}`}
    >
      {children}
    </button>
  );
}
