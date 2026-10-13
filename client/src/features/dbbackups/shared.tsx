import { RefreshCw } from "lucide-react";

export function InlineError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="rounded-2xl border border-error/30 bg-error-container/10 p-5 flex items-center justify-between gap-4 flex-wrap shadow-sm">
      <p className="text-sm font-medium text-on-surface">{message}</p>
      <button
        onClick={onRetry}
        className="inline-flex items-center gap-1.5 px-4 py-1.5 text-xs font-medium border border-outline-variant/30 rounded-full cursor-pointer text-on-surface-variant hover:text-on-surface bg-surface-container-low hover:bg-surface-container transition-colors"
      >
        <RefreshCw size={13} /> Retry
      </button>
    </div>
  );
}
