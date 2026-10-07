
export function NeonToggle({ checked, onChange, label }: { checked: boolean; onChange: () => void; label: string }) {
  return (
    <label className="flex items-center cursor-pointer relative group">
      <div className="sr-only">{label}</div>
      <input type="checkbox" className="sr-only" checked={checked} onChange={onChange} />
      <div className={`w-11 h-6 rounded-full transition-all duration-300 relative ${checked ? 'bg-cyan-500 shadow-[0_0_12px_rgba(6,182,212,0.8)]' : 'bg-gray-300 dark:bg-gray-700'}`}>
        <div className={`w-4 h-4 rounded-full bg-white absolute top-1 transition-all duration-300 ${checked ? 'left-6 shadow-[0_0_5px_rgba(255,255,255,0.8)]' : 'left-1'}`} />
      </div>
    </label>
  );
}
