export function Progress({
  value,
  className = "",
}: {
  value: number
  className?: string
}) {
  return (
    <div
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={100}
      className={`bg-secondary h-1.5 w-full overflow-hidden rounded-full ${className}`}
    >
      <div
        className="bg-primary h-full rounded-full transition-[width] duration-200"
        style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
      />
    </div>
  )
}
