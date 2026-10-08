
export function NeonToggle({ checked, onChange, label }: { checked: boolean; onChange: () => void; label: string }) {
  return (
    <label className="flex items-center cursor-pointer relative group">
      <div className="sr-only">{label}</div>
      <input type="checkbox" className="sr-only" checked={checked} onChange={onChange} />
      <div className={`w-11 h-6 rounded-full transition-colors duration-200 relative ${checked ? 'bg-accent dark:bg-ember' : 'bg-gray-300 dark:bg-zinc-700'}`}>
        <div className={`w-4 h-4 rounded-full bg-white dark:bg-black absolute top-1 transition-all duration-200 shadow-sm ${checked ? 'left-6' : 'left-1'}`} />
      </div>
    </label>
  );
}
